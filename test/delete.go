package service

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/conf"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/async_lock"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/db"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/file_lock"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/quota"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/redis"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/s3"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/lib/unamecache"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/models/dao"
	"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/utils"
)

type FileDelet struct {
	NeedUpdateProgress  bool
	EnableRedo          bool
	isRedo              bool
	Taskid              int64
	delRecords          *deleteRecords
	IsDeleteShareDir    bool
	Type                int
	Op                  int
	OwnerId             string
	ShareDir            string
	ParentShareDirMd5   uint64
	Appid               int
	DevType             string
	permanentDelete     int
	hasRecordOperid     bool
	fidProcesssed       map[int64]bool
	topFid              []int64
	currentServerMetime int64
	protectOperId       int64
	quotaAll            int64
	quotaHidden         int64
	quotaSamsung        int64
	affectRow           int
	rawPath             string
	MsgSender           func([]map[string]interface{}, []map[string]interface{}) error
	FileOperaBase
	OndupType string // 其他文件操作ondup删除时使用
}

type deleteRecord struct {
	TopFileMeta              dao.FileMeta
	BottomFileMetaCond       []string
	BottomFileMetaCondNoPath []string
}

type deleteRecords struct {
	CurrentServerMetime int64
	Worker              []string
	Records             []deleteRecord
}

func genDeleteRecordsKey(taskid int64, prefix string) string {
	return fmt.Sprintf("%sdelete-%d", prefix, taskid)
}

func getDeleteRecords(taskid int64, ctx *utils.Context) (error, *deleteRecords) {
	ufc := conf.String(conf.PCSRedisCacheTable, "pcsrecord", "ufc")
	prefix := conf.String(conf.PCSRedisCacheTable, "pcsrecord", "value")
	key := genDeleteRecordsKey(taskid, prefix)
	expire := conf.Int(conf.PCSRedisCacheTable, "pcsrecord", "expire")
	recStr, err := (&redis.Redis{Ctx: ctx}).Name(ufc).Get(key)
	if err != nil {
		ctx.L.Warn("getDeleteRecord fail err[%v]", err)
		return err, nil
	}
	if recStr == "" {
		return nil, nil
	}

	ctx.L.Info("deleteRecord not nil detail[%s]", recStr)
	var rec deleteRecords
	if errDecode := json.Unmarshal([]byte(recStr), &rec); errDecode != nil {
		ctx.L.Info("deleteRec format err[%v]", errDecode)
		return errDecode, nil
	}
	if _, errRedis := (&redis.Redis{Ctx: ctx}).Name(ufc).Expire(key, expire); errRedis != nil {
		ctx.L.Warn("set delRecKey[%s] fail err[%v]", key, errRedis)
	}
	return nil, &rec
}

func IncrDeleteSize(ctx *utils.Context, uid uint64, size int) (int, error) {
	dateYmd := time.Now().Format("20060102")
	prefix := conf.String(conf.PCSRedisCacheTable, "delete_size", "value")
	key := fmt.Sprintf("%s%s-%d", prefix, dateYmd, uid)
	ufc := conf.String(conf.PCSRedisCacheTable, "delete_size", "ufc")
	expire := conf.Int(conf.PCSRedisCacheTable, "delete_size", "expire")
	resInt, err := (&redis.Redis{Ctx: ctx}).Name(ufc).ProtoGenesisInCrByInCrBy(key, size)
	(&redis.Redis{Ctx: ctx}).Name(ufc).Expire(key, expire)
	return resInt, err
}
func cacheDeleteRecords(taskid int64, rec *deleteRecords, ctx *utils.Context) error {
	ufc := conf.String(conf.PCSRedisCacheTable, "pcsrecord", "ufc")
	prefix := conf.String(conf.PCSRedisCacheTable, "pcsrecord", "value")
	key := genDeleteRecordsKey(taskid, prefix)
	expire := conf.Int(conf.PCSRedisCacheTable, "pcsrecord", "expire")
	recStr, errjson := json.Marshal(rec)
	if errjson != nil {
		ctx.L.Warn("json encode deleteRec fail err[%v]", errjson)
		return errjson
	}
	err := (&redis.Redis{Ctx: ctx}).Name(ufc).SetEx(key, expire, string(recStr))
	if err != nil {
		ctx.L.Warn("cacheDeleteRecords fail err[%v]", err)
		return err
	}
	return nil
}

func (f *FileDelet) isAsync() (syncType int, err error) {
	if f.IsDeleteShareDir {
		return ASYNC, nil
	}

	tmpAsync := f.Async
	if tmpAsync == 0 {
		tmpAsync = f.Sync2async
	}

	if f.Type == conf.DEL_RECYCLE_FILE_TYPE {
		if tmpAsync == 1 {
			return ASYNC, nil
		}
		return SYNC, nil
	}

	if len(f.Paths) > conf.MAX_TARGETNUM {
		f.Ctx.L.Info("[count(target) is bigger than require][count:%d][max:%d]", len(f.Paths), conf.MAX_TARGETNUM)
		return 0, errors.New(conf.ERROR_FILE_COPY_OVERFLOW)
	}

	if conf.ENABLE_ASYNC == false || conf.ENABLE_ASYNC_OPERA == false || tmpAsync == SYNC {
		return SYNC, nil
	}
	if tmpAsync == 2 || (tmpAsync != 0 && len(f.Paths) >= conf.MIN_ASYNC_OUT_OPERA_COUNT_DELETE) {
		return ASYNC, nil
	}

	targetNum := conf.MIN_ASYNC_OPERA_COUNT
	file := &File{Ctx: f.Ctx}
	for index, _ := range f.Paths {
		if targetNum <= 0 {
			break
		}

		defaultFileStatu := conf.PCS_FILE_NORMAL
		if f.Type == conf.DEL_HIDDEN_FILE_TYPE {
			defaultFileStatu = conf.PCS_FILE_HIDDEN
		}
		var path string
		if v, exist := f.Paths[index]["fs_id"]; exist {
			fsid, ok := v.(int64)
			if ok == false {
				f.Ctx.L.Warn("fs_id[%v] type invalid", v)
				return 0, errors.New(conf.ERROR_PARAM_ERROR)
			}

			tmpFileMeta, errGetFile := dao.NewFileMetaView(f.Ctx).GetFileListByFsId(f.Uid, []int64{fsid}, conf.PCS_FILE_MOST)
			if errGetFile != nil {
				f.Ctx.L.Warn("[GetFileListByFsId failed][fsId:%d][err:%v]", fsid, errGetFile)
				return 0, errGetFile
			}
			if len(tmpFileMeta) <= 0 {
				if f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
					continue
				}
				f.Ctx.L.Warn("[GetFileListByFsId failed][fsId:%v][err:not found file]", fsid)
				return 0, errors.New(conf.ERROR_FILE_FSID_INVALID)
			}
			if tmpFileMeta[0].IsDir == 0 {
				targetNum--
				continue
			}

			parts := strings.Split(tmpFileMeta[0].Path, ":")
			if len(parts) <= 1 {
				f.Ctx.L.Warn("[GetFileListByFsId failed][fsId:%v][path:%v][err:path invalid]", fsid, tmpFileMeta[0].Path)
				return 0, errors.New(conf.ERROR_FILE_FSID_INVALID)
			}
			dealPath, errFrom := file.DealPath(f.Uid, parts[1], true)
			if errFrom != nil {
				return 0, errFrom
			}
			path = dealPath["path"].(string)

			if tmpFileMeta[0].IsDelete == conf.PCS_FILE_HIDDEN || tmpFileMeta[0].ExtentTinyint2 == 1 {
				defaultFileStatu = conf.PCS_FILE_HIDDEN
			} else {
				defaultFileStatu = tmpFileMeta[0].IsDelete
			}
		} else if vPath, existPath := f.Paths[index]["path"]; existPath {
			var ok bool
			path, ok = vPath.(string)
			if ok == false {
				f.Ctx.L.Warn("path[%v] type invalid", vPath)
				return 0, errors.New(conf.ERROR_PARAM_ERROR)
			}
			dealPath, errFrom := file.DealPath(f.Uid, path, true)
			if errFrom != nil {
				return 0, errFrom
			}
			path = dealPath["path"].(string)

			tmpFileStatu := conf.PCS_FILE_MOST
			if f.Op != conf.DEL_PERMANENT_OP {
				tmpFileStatu = defaultFileStatu
			}

			tmpFileMeta, errGetFile := dao.NewFileMetaView(f.Ctx).GetFileMeta(f.Uid, path, tmpFileStatu, false)
			if errGetFile != nil {
				return 0, errGetFile
			}
			if tmpFileMeta == nil {
				if f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
					continue
				}
				return 0, errors.New(conf.ERROR_FILE_NOT_EXIST)
			}

			if tmpFileMeta.IsDir == 0 {
				targetNum--
				continue
			}
			if tmpFileMeta.IsDelete == conf.PCS_FILE_HIDDEN || tmpFileMeta.ExtentTinyint2 == 1 {
				defaultFileStatu = conf.PCS_FILE_HIDDEN
			} else {
				defaultFileStatu = tmpFileMeta.IsDelete
			}
		} else {
			f.Ctx.L.Warn("Paths item [%v] invalid", f.Paths[index])
			return 0, errors.New(conf.ERROR_PARAM_ERROR)
		}

		if defaultFileStatu == conf.PCS_FILE_RECYCLED_TOP {
			defaultFileStatu = conf.PCS_FILE_RECYCLED
		}

		if meta, errCount := dao.NewFileMetaView(f.Ctx).CountDirSize(f.Uid, path, defaultFileStatu); errCount == nil {
			if len(meta) > 0 {
				targetNum -= int(meta[0].SubFileNums)
			}
		} else {
			targetNum = -1
			break
		}
	}

	if tmpAsync == 1 {
		if targetNum > 0 {
			return SYNC, nil
		}
	}
	return ASYNC, nil
}

func (f *FileDelet) BeforeMulti() (syncType int, err error) {
	res, err := f.isAsync()
	if err != nil {
		return res, err
	}

	if f.Context != "" && len(f.Paths) > 0 {
		path, _ := f.Paths[0]["path"].(string)
		var pathMd5 uint64
		if pathMd5, err = (&ShareDir{f.Ctx}).HasParentSharedir(f.Uid, path); err == nil {
			f.ParentShareDirMd5 = pathMd5
		}
	}

	return res, err
}

func (f *FileDelet) FileMultiAsync() (int64, error) {
	msgMeta := map[string]interface{}{
		"cb_channelid": f.CbChannelId,
		"cb_param":     f.CbParam,
		"sync2async":   f.Sync2async,
	}

	if extra, ok := f.Ctx.Session["extra_message_data"]; ok {
		msgMeta["extra_message_data"] = extra
	}

	f.Ctx.Session[conf.METHOD] = conf.ASYNC_DELETE_MESSAGE_METHOD
	switch f.Type {
	case conf.DEL_HIDDEN_FILE_TYPE:
		msgMeta["type"] = conf.DEL_HIDDEN_FILE_TYPE_STR
	case conf.DEL_RECYCLE_FILE_TYPE:
		msgMeta["type"] = conf.DEL_RECYCLE_FILE_TYPE_STR
	default:
		msgMeta["type"] = conf.DEL_NORMAL_FILE_TYPE_STR
	}

	switch f.Op {
	case conf.DEL_PERMANENT_OP:
		msgMeta["op"] = conf.DEL_PERMANENT_OP_STR
	default:
		msgMeta["op"] = conf.DEL_RECYCLED_OP_STR
	}
	if f.IsDeleteShareDir {
		if f.OwnerId == "" || f.OperaId == 0 || f.ShareDir == "" {
			return 0, errors.New(conf.ERROR_PARAM_ERROR)
		}
		msgMeta["share_dir"] = f.ShareDir
		msgMeta["mode"] = "sharedir"
		msgMeta["owner_id"] = f.OwnerId
		msgMeta["oper_id"] = fmt.Sprintf("%d", f.OperaId)
		msgMeta["target"] = []map[string]interface{}{}
	} else {
		msgMeta["target"] = f.Paths
	}

	if f.Context != "" {
		msgMeta["context"] = f.Context
	}

	if f.ProductType == conf.PCS_ENTERPRISESPACE_PRODUCT {
		msgMeta["oper_id"] = fmt.Sprintf("%d", f.OperaId)
	}
	msgMeta["skipnotexist"] = f.SkipNotExist
	return (&File{Ctx: f.Ctx}).DealMultiAsync(msgMeta, 0)
}

func (f *FileDelet) lockForDelete() bool {
	if f.isRedo {
		f.unlockForDelete()
	}

	res := true
	lockedPath := map[string]string{}

	for index, _ := range f.Paths {
		val, exist := f.Paths[index]["path"]
		if exist == false {
			continue
		}
		path, ok := val.(string)
		if ok == false {
			continue
		}
		pathMd5 := fmt.Sprintf("%x", md5.Sum([]byte(path)))

		al := (&async_lock.Asynclock{Ctx: f.Ctx})
		if al.DealGlobalLockWithExtraPart(f.Uid, async_lock.DELETE_LOCK, pathMd5) == false {
			f.Ctx.L.Warn("lock for delete failed, path:%s pathMd5:%s", path, pathMd5)
			res = false
			break
		} else {
			lockedPath[pathMd5] = path
		}
	}

	if res == false {
		al := (&async_lock.Asynclock{Ctx: f.Ctx})
		for pmd5, path := range lockedPath {
			status := al.DealGlobalUnLockWithExtraPart(f.Uid, async_lock.DELETE_LOCK, pmd5)
			if status != async_lock.SUCC {
				f.Ctx.L.Warn("unlock for delete failed, path:%s pathMd5:%s status:%d", path, pmd5, status)
			}
		}
	}

	return res
}

func (f *FileDelet) unlockForDelete() {
	for index, _ := range f.Paths {
		al := (&async_lock.Asynclock{Ctx: f.Ctx})
		val, exist := f.Paths[index]["path"]
		if exist == false {
			continue
		}
		path, ok := val.(string)
		if ok == false {
			continue
		}
		pathMd5 := fmt.Sprintf("%x", md5.Sum([]byte(path)))
		status := al.DealGlobalUnLockWithExtraPart(f.Uid, async_lock.DELETE_LOCK, pathMd5)
		if status != async_lock.SUCC {
			f.Ctx.L.Warn("unlock for delete failed, path:%s pathMd5:%s status:%d", path, pathMd5, status)
		}
	}
}

func (f *FileDelet) genSubDelMeta(file *dao.FileMeta) map[string]interface{} {
	message := make(map[string]interface{})
	message["isdir"] = file.IsDir
	message["size"] = file.Size
	message["path"] = file.Path
	message["fs_id"] = file.FsId
	if 1 != file.IsDir {
		message["object"] = s3.GetMd5(s3.GetBlockList(file.S3Handle))
	}
	message["status"] = file.Status
	message["permanent"] = f.permanentDelete
	message["category"] = file.Category
	message["lctime"] = file.LocalCtime
	message["lmtime"] = file.LocalMtime
	message["sctime"] = file.ServerCtime
	if file.RealServerMtime2 > 0 {
		message["smtime"] = file.RealServerMtime2
	} else if file.RealServerMtime > 0 {
		message["smtime"] = file.RealServerMtime
	} else {
		message["smtime"] = file.ServerMtime
	}
	message["isdelete"] = file.IsDelete
	return message
}

func (f *FileDelet) delFiles(fsids []int64, param map[string]interface{}, valueAndFiled map[string]string) error {

	if isdelete, ok := param["isdelete"]; ok && isdelete == -2 {
		param["isdelete"] = 0
		param["extent_int2"] = 1
	}

	dbQuery := db.New(dao.NewFileMetaView(f.Ctx)).SetCond("user_id=%d", f.Uid)
	if len(fsids) == 1 {
		dbQuery.AndCond("fs_id = %d", fsids[0])
	} else if len(fsids) > 1 {
		dbQuery.AndCond("fs_id in (%s)", utils.ImplodeInt64(",", fsids))
	}
	for k, v := range param {
		dbQuery.FieldValue(k, v)
	}
	for k, v := range valueAndFiled {
		dbQuery.FieldRawValue(k, v)
	}
	retry := 2
	for i := 0; true; i++ {
		_, err := dbQuery.Update()
		if err != nil {
			f.Ctx.L.Warn("GetUpdateFileMeta fail try[%d] err[%v]", i, err)
			if i >= retry {
				return err
			}
		} else {
			return nil
		}
	}
	return nil
}

func (f *FileDelet) batchDelete(files []*dao.FileMeta, mainFsidDict bool, memberId int) error {
	if len(files) == 0 {
		return nil
	}

	var quotaAll int64
	var quotaSamsung int64
	var quotaHiddenAll int64

	var bigpipeSubMsgMetaList []map[string]interface{}
	(&redis.Cache{Ctx: f.Ctx}).SetUidToRedis(f.Uid)
	inotify := &Notify{Event: conf.ACTION_DEL, Type: conf.ACTION_TYPE_SUB, Ctx: f.Ctx}

	for _, file := range files {
		if _, exist := f.fidProcesssed[file.FsId]; exist == true {
			continue
		}

		if file.IsDelete == conf.PCS_FILE_RECYCLED_TOP {
			f.topFid = append(f.topFid, file.FsId)
		}
		f.fidProcesssed[file.FsId] = true
		if file.IsDelete == conf.PCS_FILE_NORMAL || file.IsDelete == conf.PCS_FILE_HIDDEN {
			quotaAll += int64(file.Size)
			if (&File{Ctx: f.Ctx}).IsSafeBox(f.Uid, file.Path) {
				quotaHiddenAll += int64(file.Size)
			}
			if file.Videotag == 1 {
				quotaSamsung += int64(file.Size)
			}
			if mainFsidDict == false {
				meta := f.genSubDelMeta(file)
				bigpipeSubMsgMetaList = append(bigpipeSubMsgMetaList, meta)
			}
		}
		inotify.PushSimpleItem(file.Path, file.FsId, file.IsDir)
	}

	if len(bigpipeSubMsgMetaList) != 0 {
		err := f.MsgSender(nil, bigpipeSubMsgMetaList)
		if err != nil {
			f.Ctx.L.Warn("sendMsg err[%v].", err)
		}
	}

	isRecycled := true
	if len(files) > 0 {
		if files[0].IsDelete == conf.PCS_FILE_NORMAL && files[0].ExtentInt2 == 0 {
			isRecycled = false
		}
		if f.ProductType == conf.PCS_WORKSPACE_PRODUCT && files[0].IsDelete == conf.PCS_FILE_NORMAL && files[0].ExtentInt2 == 1 && f.Op != conf.DEL_PERMANENT_OP {
			isRecycled = false
		}
		if f.ProductType == conf.PCS_ENTERPRISESPACE_PRODUCT && files[0].IsDelete == conf.PCS_FILE_NORMAL && files[0].ExtentInt2 == 1 && f.Op != conf.DEL_PERMANENT_OP {
			isRecycled = false
		}
	}

	valueAndFiled := make(map[string]string)
	valueAndFiled["delete_fs_id"] = "fs_id"
	valueAndFiled["path_md5"] = utils.PathMd5Sql(`concat(parent_path,"/",server_filename)`)

	arrParam := map[string]interface{}{}
	arrParam["extent_int3"] = f.OperaId
	arrParam["share"] = 0
	if isRecycled == false {
		if !utils.UseDbTimeForServerMTime() {
			arrParam["server_mtime"] = f.currentServerMetime
		} else {
			valueAndFiled["server_mtime"] = `unix_timestamp()`
		}
		arrParam["real_server_mtime"] = f.currentServerMetime
	}

	if f.DevType == "mac" {
		arrParam["extent_tinyint2"] = 1
	}
	if f.DevType == "pc" {
		arrParam["delete_type"] = 1
	}

	topFidSet := map[int64]bool{}
	var topFidUpdate []int64
	var bottomFidUpdate []int64
	var topFiles []*dao.FileMeta
	var bottomFiles []*dao.FileMeta

	for _, fid := range f.topFid {
		topFidSet[fid] = true
	}
	for index, _ := range files {
		if _, exist := topFidSet[files[index].FsId]; exist == true {
			topFiles = append(topFiles, files[index])
			topFidUpdate = append(topFidUpdate, files[index].FsId)
		} else {
			bottomFiles = append(bottomFiles, files[index])
			bottomFidUpdate = append(bottomFidUpdate, files[index].FsId)
		}
	}

	operId := f.currentServerMetime - f.currentServerMetime%conf.FILE_PROTECT_DAY
	if isRecycled == false && len(topFidUpdate) != 0 && f.hasRecordOperid == false {
		f.hasRecordOperid = true
		if (&redis.Cache{Ctx: f.Ctx}).RecordOperid(f.Uid, operId) == false {
			return errors.New(conf.ERROR_FILE_DELETE_FAILED)
		}
	}

	if !isRecycled && f.Op != conf.DEL_PERMANENT_OP {
		isWorkspace := f.Ctx.Session[conf.IS_WORKSPACE].(bool)
		// f.Ctx.Session[conf.WORKSPACE_NEW_VERSION] = (&WorkspaceService{Ctx: f.Ctx}).Genrevision()

		if len(topFidUpdate) != 0 && len(topFiles) != 0 {
			arrParam["isdelete"] = conf.PCS_FILE_RECYCLED_TOP
			arrParam["extent_tinyint5"] = conf.PCS_FILE_PROTECT_TOP
			arrParam["protect_operid"] = f.protectOperId
			if err := f.delFiles(topFidUpdate, arrParam, valueAndFiled); err != nil {
				if isWorkspace {
					f.Ctx.L.Warn("[workspace_request][possible dirty data][delFiles failed][uid:%d][err:%v]", f.Uid, err)
				}
				return err
			}
			delete(arrParam, "protect_operid")

			if (&File{Ctx: f.Ctx}).IsWorkspace(f.Uid, topFiles[0].Path) {
				ws := &WorkspaceService{Ctx: f.Ctx}
				errIP := ws.InsertRepositories(&InsertRepositoriesArgs{
					Uid:       f.Uid,
					Metas:     topFiles,
					Reversion: f.Ctx.Session[conf.WORKSPACE_NEW_VERSION],
					OpType:    conf.WORKSPACE_OP_DELETED,
					OpFunc:    0,
					BatchSize: 2000,
				})
				if errIP != nil {
					f.Ctx.L.Warn("[workspace_request][possible dirty data][generally not][InsertRepositories failed][uid:%d][err:%v]", f.Uid, errIP)
					return errIP
				}
			}
		}

		if len(bottomFidUpdate) != 0 && len(bottomFiles) != 0 {
			arrParam["isdelete"] = conf.PCS_FILE_RECYCLED
			arrParam["extent_tinyint5"] = conf.PCS_FILE_PROTECT
			if err := f.delFiles(bottomFidUpdate, arrParam, valueAndFiled); err != nil {
				if isWorkspace {
					f.Ctx.L.Warn("[workspace_request][possible dirty data][delFiles failed][uid:%d][err:%v]", f.Uid, err)
				}
				return err
			}

			if (&File{Ctx: f.Ctx}).IsWorkspace(f.Uid, bottomFiles[0].Path) {
				ws := &WorkspaceService{Ctx: f.Ctx}
				errIP := ws.InsertRepositories(&InsertRepositoriesArgs{
					Uid:       f.Uid,
					Metas:     bottomFiles,
					Reversion: f.Ctx.Session[conf.WORKSPACE_NEW_VERSION],
					OpType:    conf.WORKSPACE_OP_DELETED,
					OpFunc:    0,
					BatchSize: 2000,
				})
				if errIP != nil {
					f.Ctx.L.Warn("[workspace_request][possible dirty data][generally not][InsertRepositories failed][uid:%d][err:%v]", f.Uid, errIP)
					return errIP
				}
			}
		}
	} else {
		arrParam["isdelete"] = conf.PCS_FILE_RECYCLED
		if f.Op == conf.DEL_PERMANENT_OP || f.Type == conf.DEL_RECYCLE_FILE_TYPE || f.Type == conf.DEL_HIDDEN_FILE_TYPE {
			arrParam["isdelete"] = conf.PCS_FILE_DELETED
		}
		if len(topFidUpdate) != 0 {
			arrParam["extent_tinyint5"] = conf.PCS_FILE_PROTECT_TOP
			//不再关注隐藏文件类型
			//是否增加protect_operid根据删除至回收站时决定
			//是否刷新protect_operid字段由下面判断决定

			var updateFsIds []int64
			var needUpdateProtect []int64
			fsIdMap := make(map[int64]*dao.FileMeta)
			for _, v := range files {
				fsIdMap[v.FsId] = v
			}
			for _, v := range topFidUpdate {
				if meta, ok := fsIdMap[v]; ok && meta.ProtectOperid > int(f.currentServerMetime) {
					needUpdateProtect = append(needUpdateProtect, meta.FsId)
				} else {
					updateFsIds = append(updateFsIds, meta.FsId)
				}
			}
			f.Ctx.L.Info("[delete:update top file][uid:%d][need_delete:%v][updateFsIds:%v][needUpdateProtect:%v]", f.Uid, topFidUpdate, updateFsIds, needUpdateProtect)
			if len(updateFsIds) > 0 {
				if err := f.delFiles(updateFsIds, arrParam, valueAndFiled); err != nil {
					return err
				}
			}
			if len(needUpdateProtect) > 0 {
				arrParam["protect_operid"] = f.currentServerMetime
				if err := f.delFiles(needUpdateProtect, arrParam, valueAndFiled); err != nil {
					return err
				}
			}
			delete(arrParam, "protect_operid")
		}
		if len(bottomFidUpdate) != 0 {
			arrParam["extent_tinyint5"] = conf.PCS_FILE_PROTECT
			if err := f.delFiles(bottomFidUpdate, arrParam, valueAndFiled); err != nil {
				return err
			}
		}
	}

	if len(topFidUpdate) != 0 && isRecycled == false {
		(&redis.Cache{Ctx: f.Ctx}).RecordProtectCount(f.Uid, operId, len(topFidUpdate))
	}

	if quotaAll != 0 {
		var err error
		if utils.InIntArray(conf.ExtraApp, f.Appid) {
			_, err = (&quota.Quota{Ctx: f.Ctx}).AdjUsed(0-quotaAll, f.Uid, "", memberId, f.Appid)
		} else {
			_, err = (&quota.Quota{Ctx: f.Ctx}).AdjUsed(0-quotaAll, f.Uid, "", memberId, 0)
		}
		if err != nil {
			f.Ctx.L.Warn("adjust quota fail size[%d], uid[%d], memeberId[%v] appid[%d] err[%v]", 0-quotaAll, f.Uid, memberId, f.Appid, err)
		}
	}
	if quotaHiddenAll != 0 {
		err := (&File{Ctx: f.Ctx}).AdjHiddenUsed(f.Uid, uint64(0)-uint64(quotaHiddenAll))
		if err != nil {
			f.Ctx.L.Warn("adj hidden quota fail err[%v]", err)
		}
	}
	if quotaSamsung != 0 {
		_, err := (&quota.Quota{Ctx: f.Ctx}).AdjUsed(0-quotaAll, f.Uid, "", memberId, 23029418)
		if err != nil {
			f.Ctx.L.Warn("adjust samsung quota fail size[%d], uid[%d], memeberId[%v] appid[%d] err[%v]", 0-quotaSamsung, f.Uid, memberId, f.Appid, err)
		}
	}

	f.quotaAll += quotaAll
	f.quotaHidden += quotaHiddenAll
	f.quotaSamsung += quotaSamsung

	inotify.Finish()

	return nil
}

func (f *FileDelet) fetchOneFileMetaByFsId(fsid int64, isDelete int) (*dao.FileMeta, error) {
	fileList, err := dao.NewFileMetaView(f.Ctx).DoGetFileListByFsId(f.Uid, []int64{fsid}, isDelete, true)
	if err != nil {
		f.Ctx.L.Warn("fetchOneFileMetaByFsId fail fsid[%d] uid[%d] isdelete[%d] err[%v]", fsid, f.Uid, isDelete, err)
		return nil, err
	}
	if len(fileList) == 0 {
		return nil, errors.New(conf.ERROR_FILE_NOT_EXIST)
	}
	fileItem := fileList[0]
	return fileItem, nil
}

func (f *FileDelet) fetchOneFileMetaByPath(path string, isDelete int) (*dao.FileMeta, error) {
	pathMap, err := (&File{Ctx: f.Ctx}).DealPath(f.Uid, path, true)
	if err != nil {
		f.Ctx.L.Warn("[deal path fail] [err:%v]", err)
		return nil, err
	}
	path = pathMap["path"].(string)
	fileMeta, err := dao.NewFileMetaView(f.Ctx).GetFileMeta(f.Uid, path, isDelete, false)
	if err != nil {
		f.Ctx.L.Warn("[get filemeta fail] [err:%v]", err)
		return nil, err
	}

	if fileMeta == nil {
		f.Ctx.L.Warn("[get filemeta empty]")
		return nil, errors.New(conf.ERROR_FILE_NOT_EXIST)
	}
	return fileMeta, nil
}

func (f *FileDelet) fetchShareDir() (*dao.FileMeta, []*dao.FileMeta, error) {
	var res, all []*dao.FileMeta

	topFile, err := f.fetchOneFileMetaByPath(f.ShareDir, conf.PCS_FILE_NORMAL)
	if err != nil {
		f.Ctx.L.Error("get share dir err[%v]", err)
		return nil, nil, err
	}
	if topFile.IsDir == 0 {
		f.Ctx.L.Warn("share dir fileType not right")
		return nil, nil, errors.New(conf.ERROR_PARAM_ERROR)
	}

	path := utils.Addslashes(topFile.Path)
	likePath := utils.Strtr(topFile.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
	shareDirCond := []string{
		fmt.Sprintf("user_id=%d", f.Uid),
		utils.WhereDeletedSql(conf.PCS_FILE_NORMAL),
		"isdir=1",
		fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path),
		fmt.Sprintf("extent_int3=%d", f.OperaId),
	}
	if all, err = f.fetchFiles(-1, -1, shareDirCond, ""); err != nil {
		f.Ctx.L.Warn("fetch sub shareDir err[%v]", err)
		return topFile, res, err
	}

	sort.Slice(
		all,
		func(i, j int) bool {
			return all[i].Path < all[j].Path
		},
	)
	for indexAll, _ := range all {
		isSub := false
		for indexRes, _ := range res {
			if strings.Index(all[indexAll].Path, res[indexRes].Path+"/") == 0 {
				isSub = true
				break
			}
		}
		if !isSub {
			res = append(res, all[indexAll])
		}
	}
	return topFile, res, nil
}

func (f *FileDelet) genShareSubFileCond() ([]string, error) {
	pathMap, err := (&File{Ctx: f.Ctx}).DealPath(f.Uid, f.ShareDir, true)
	if err != nil {
		f.Ctx.L.Warn("[deal path fail] [err:%v]", err)
		return nil, err
	}
	path := pathMap["path"].(string)
	tpath := utils.Addslashes(path)
	likePath := utils.Strtr(path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
	return []string{
		fmt.Sprintf("user_id=%d", f.Uid),
		utils.WhereDeletedSql(conf.PCS_FILE_NORMAL),
		"isdir=0",
		fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, tpath),
		fmt.Sprintf("extent_int3=%d", f.OperaId),
	}, nil
}

func (f *FileDelet) fetchFiles(start, offset int, cond []string, forceIndex string) ([]*dao.FileMeta, error) {
	if len(cond) == 0 {
		return []*dao.FileMeta{}, nil
	}
	fv := dao.NewFileMetaView(f.Ctx)
	d := db.New(fv)
	d.ForceQueryMaster(true)
	d.Field(fv.MkField("all"))
	for index, _ := range cond {
		d.SetCond(cond[index])
	}

	if start >= 0 && offset >= 0 {
		d.Limit(start, offset)
	}

	if len(forceIndex) > 0 {
		d.AddForceIndex(forceIndex)
	}

	fileMetas, err := d.SelectToBuild()
	if err != nil {
		return nil, err
	}
	var res []*dao.FileMeta
	for _, fileMeta := range fileMetas {
		res = append(res, fileMeta.(*dao.FileMeta))
	}
	return res, nil
}

func (f *FileDelet) checkBaseRevision(paths []map[string]interface{}, path string) (uint64, map[string]interface{}, error) {
	var baseRevision int64

	var tmpItem map[string]interface{}
	for idx, item := range paths {
		if str, ok := item["path"].(string); ok && str == path {
			tmpItem = paths[idx]
			if revision, ok := item["base_revision"].(int64); ok {
				baseRevision = revision
				break
			}
			if revision, ok := item["base_revision"].(float64); ok {
				baseRevision = int64(revision)
				break
			}
		}
	}

	// 请求里传递了base_revision，但未匹配到path，异常情况。日志记录，不额外处理。
	if isLocal, ok := f.Ctx.Session["is_local"]; ok && isLocal == 0 && (tmpItem == nil || len(tmpItem["path"].(string)) == 0) {
		f.Ctx.L.Warn("[is local delete but not match path, maybe special char][paths:%v][record:%v]", paths, path)
	}

	if baseRevision < 0 {
		f.Ctx.L.Warn("[base revision is less than 0] [baseRevision:%v]", baseRevision)
		return 0, tmpItem, errors.New(conf.ERROR_PARAM_ERROR)
	}
	if baseRevision == 0 {
		return 0, nil, nil
	}
	file := &File{Ctx: f.Ctx}
	if !file.IsWorkspace(f.Uid, path) {
		return 0, nil, nil
	}

	path = utils.StrReplace(strconv.FormatUint(f.Uid, 10)+":", "", path, -1)
	nowRevision, err := (&WorkspaceService{f.Ctx}).GetMaxrevisionByPath(f.Uid, path)
	if err != nil {
		f.Ctx.L.Warn("[get revision by path failed] [path:%v] [err:%v]", path, err)
		return 0, tmpItem, err
	}
	if nowRevision != uint64(baseRevision) {
		f.Ctx.L.Warn("[check revision failed] [nowRevision:%d] [baseRevision:%d]", nowRevision, baseRevision)
		return 0, tmpItem, errors.New(conf.ERROR_ENTERPRISE_SYNC_REVISION_CONFLICT)
	}
	return nowRevision, nil, nil
}

func (f *FileDelet) getFileNum(cond []string) (int, error) {
	d := db.New(dao.NewFileMetaView(f.Ctx)).Field("count(*) as num")
	for _, c := range cond {
		d.SetCond(c)
	}
	dbRes, err := d.Select()
	if err != nil || len(dbRes) == 0 {
		return 0, errors.New(conf.ERROR_DB_QUERY_ERROR)
	}
	return utils.AtoiWithoutError(dbRes[0]["num"]), nil
}

func (f *FileDelet) hierarchyProcess(cond, condNoPath []string, topFileMeta *dao.FileMeta) (int, error) {
	f.Ctx.StatusStart()
	f.Ctx.StatusEnd()
	if len(cond) < 1 || len(condNoPath) < 1 {
		return f.process(cond, []*dao.FileMeta{topFileMeta}, false)
	}
	condAffecteNum := 0

	isRecycled := true
	if topFileMeta != nil {
		if topFileMeta.IsDelete == conf.PCS_FILE_NORMAL && topFileMeta.ExtentInt2 == 0 {
			isRecycled = false
		}
		if f.ProductType == conf.PCS_WORKSPACE_PRODUCT && topFileMeta.IsDelete == conf.PCS_FILE_NORMAL && topFileMeta.ExtentInt2 == 1 && f.Op != conf.DEL_PERMANENT_OP {
			isRecycled = false
		}
		if f.ProductType == conf.PCS_ENTERPRISESPACE_PRODUCT && topFileMeta.IsDelete == conf.PCS_FILE_NORMAL && topFileMeta.ExtentInt2 == 1 && f.Op != conf.DEL_PERMANENT_OP {
			isRecycled = false
		}
	}
	f.Ctx.L.Info("[process hierarchy] [isRecycled: %v]", isRecycled)

	// 新建目录结构
	start := time.Now().Unix()
	fsIdDeque, dirMap, err := f.copyDirs(condNoPath, topFileMeta, isRecycled)
	if err != nil {
		f.Ctx.L.Warn("[copy dirs fail] [err: %v]", err)
		return 0, err
	}
	f.Ctx.L.Info("[copy dirs] [cost: %v]", time.Now().Unix()-start)

	// update 文件
	start = time.Now().Unix()
	tmpCond := append(cond, "isdir=0")
	affect, err := f.process(tmpCond, nil, false)
	condAffecteNum += affect
	if err != nil {
		f.Ctx.L.Warn("[update file to del fail] [err: %v]", err)
		return condAffecteNum, err
	}
	f.Ctx.L.Info("[update file] [cost: %v]", time.Now().Unix()-start)

	// 交换fsid
	start = time.Now().Unix()
	affect, err = f.exchangeAndDelDirs(topFileMeta, fsIdDeque, dirMap, isRecycled)
	condAffecteNum += affect
	if err != nil {
		f.Ctx.L.Warn("[exchange and del dir fail] [err: %v]", err)
		return condAffecteNum, err
	}
	f.Ctx.L.Info("[exchange and del dir] [cost: %v]", time.Now().Unix()-start)

	// update 孤岛文件、文件夹
	start = time.Now().Unix()
	affect, err = f.process(cond, nil, false)
	condAffecteNum += affect
	if err != nil {
		f.Ctx.L.Warn("[update island to del fail] [err: %v]", err)
		return condAffecteNum, err
	}
	f.Ctx.L.Info("[update island file] [cost: %v]", time.Now().Unix()-start)

	// update to many file
	if condAffecteNum > conf.UPDATE_TO_MANY_FILE {
		f.Ctx.L.Warn("update to many file num[%d]", condAffecteNum)
	}

	// 保存子文件总容量等信息，只有topFileMeta为文件夹时会执行到此步骤，故 affectRow-1 容量不需要减topFileMeta
	extentString1 := utils.GenerateExtentString1(f.quotaAll, f.quotaSamsung, f.quotaHidden, condAffecteNum-1)
	if len(extentString1) > 0 {
		updateKV := map[string]interface{}{
			"extent_string1": extentString1,
		}
		aff, err1 := dao.NewFileMetaView(f.Ctx).UpdateMeta(topFileMeta.UserId, topFileMeta.FsId, updateKV)
		if aff < 0 || err1 != nil {
			f.Ctx.L.Warn("[update extent_string1 fail] [aff:%v] [err:%v]", aff, err)
		}
	}
	f.affectRow = condAffecteNum
	return condAffecteNum, nil
}

func (f *FileDelet) exchangeAndDelDirs(topFileMeta *dao.FileMeta, fsIdDeque *utils.Deque, dirMap map[int64]*dao.FileMeta, isRecycled bool) (int, error) {
	f.Ctx.StatusStart()
	defer f.Ctx.StatusEnd()

	uid := topFileMeta.UserId
	var bigpipeSubMsgMetaList []map[string]interface{}
	inotify := &Notify{Event: conf.ACTION_DEL, Type: conf.ACTION_TYPE_SUB, Ctx: f.Ctx}
	affect := 0
	for fsIdDeque.Size() > 0 {
		var fromFsIds []int64
		var toFsIds []int64
		var metas []*dao.FileMeta
		for i := 0; i < conf.RUN_SQL_SIZE && fsIdDeque.Size() > 0; i++ {
			pair, _ := fsIdDeque.PopLast().(*FsIdPair)
			if pair == nil {
				f.Ctx.L.Warn("[fs id pair nil return]")
				return affect, errors.New(conf.ERROR_FILE_DELETE_FAILED)
			}
			fromFsIds = append(fromFsIds, pair.fromFsId)
			toFsIds = append(toFsIds, pair.toFsId)
			if _, ok := dirMap[pair.fromFsId]; ok {
				metas = append(metas, dirMap[pair.fromFsId])
			} else {
				f.Ctx.L.Warn("[file meta not exist] [fsid: %v]", pair.fromFsId)
			}
		}

		// delete from
		updateKV := make(map[string]interface{})
		updatePairRaw := []*dao.Pair{
			{Field: "fs_id", RawValue: utils.NewFsIdSql(int(f.currentServerMetime))},
			{Field: "delete_fs_id", RawValue: "fs_id"},
			{Field: "server_mtime", RawValue: "unix_timestamp()"},
		}
		updateKV["isdelete"] = 1
		updateKV["protect_operid"] = 0
		updateKV["extent_tinyint5"] = 0
		updateKV["real_server_mtime"] = int(f.currentServerMetime)
		_, err := dao.NewFileMetaView(f.Ctx).UpdateMetas(uid, fromFsIds, updateKV, updatePairRaw)
		if err != nil {
			f.Ctx.L.Warn("[del from dir fail] [err: %s]", err)
			return affect, err
		}
		affect += len(fromFsIds)

		// update to fs_id
		updatePairRaw = []*dao.Pair{
			{Field: "fs_id", RawValue: "extent_int6"},
			{Field: "delete_fs_id", RawValue: "extent_int6"},
			{Field: "extent_int6", RawValue: "extent_int6 & 0"},
			// {Field: "server_mtime", RawValue: "unix_timestamp()"},
		}
		_, err = dao.NewFileMetaView(f.Ctx).UpdateMetas(uid, toFsIds, nil, updatePairRaw)
		if err != nil {
			f.Ctx.L.Warn("[update to fs_id fail] [err: %s]", err)
			return affect, err
		}

		if len(metas) < 1 {
			f.Ctx.L.Warn("[metas is empty]")
			continue
		}

		// message
		for _, file := range metas {
			if file.IsDelete == conf.PCS_FILE_NORMAL || file.IsDelete == conf.PCS_FILE_HIDDEN {
				if file.FsId != topFileMeta.FsId {
					meta := f.genSubDelMeta(file)
					bigpipeSubMsgMetaList = append(bigpipeSubMsgMetaList, meta)
				}
			}
			inotify.PushSimpleItem(file.Path, file.FsId, file.IsDir)
		}

		// workspace repository
		if len(metas) > 0 && !isRecycled && f.Op != conf.DEL_PERMANENT_OP {
			if (&File{Ctx: f.Ctx}).IsWorkspace(f.Uid, metas[0].Path) {
				f.Ctx.Session[conf.WORKSPACE_NEW_VERSION] = (&WorkspaceService{Ctx: f.Ctx}).Genrevision()
				ws := &WorkspaceService{Ctx: f.Ctx}
				errIP := ws.InsertRepositories(&InsertRepositoriesArgs{
					Uid:       f.Uid,
					Metas:     metas,
					Reversion: f.Ctx.Session[conf.WORKSPACE_NEW_VERSION],
					OpType:    conf.WORKSPACE_OP_DELETED,
					OpFunc:    0,
					BatchSize: 2000,
				})
				if errIP != nil {
					f.Ctx.L.Warn("[workspace_request][possible dirty data][generally not][InsertRepositories failed][uid:%d][err:%v]", f.Uid, errIP)
					return affect, err
				}
			}
		}
	}
	if len(bigpipeSubMsgMetaList) != 0 {
		err := f.MsgSender(nil, bigpipeSubMsgMetaList)
		if err != nil {
			f.Ctx.L.Warn("sendMsg err[%v].", err)
		}
	}
	inotify.Finish()
	return affect, nil
}

func (f *FileDelet) copyDirs(cond []string, topFileMeta *dao.FileMeta, isRecycled bool) (*utils.Deque, map[int64]*dao.FileMeta, error) {
	f.Ctx.StatusStart()
	defer f.Ctx.StatusEnd()

	deque := utils.NewDeque()
	fsIdDeque := utils.NewDeque()
	uid := topFileMeta.UserId
	condAffecteNum := 0
	batchSize := conf.SELECT_BATCH_SIZE
	dirMap := make(map[int64]*dao.FileMeta)
	dirMap[topFileMeta.FsId] = topFileMeta

	// insert tmp top
	fsIdPairs, err := f.insertDir(uid, []*dao.FileMeta{topFileMeta}, true, isRecycled)
	if err != nil {
		f.Ctx.L.Warn("[insert tmp top dir fail] [err: %s]", err)
		return fsIdDeque, dirMap, err
	}
	condAffecteNum++
	for _, pair := range fsIdPairs {
		fsIdDeque.AddTail(pair)
	}
	(&redis.Cache{Ctx: f.Ctx}).SetUidToRedis(f.Uid)

	// insert tmp sub
	deque.AddTail(topFileMeta.Path)
	for deque.Size() > 0 {
		var paths []string
		for deque.Size() > 0 {
			rawPath, _ := deque.PopFirst().(string)
			path := utils.Addslashes(rawPath)
			paths = append(paths, fmt.Sprintf(`"%s"`, path))
		}
		pathsArr := utils.ArrayChunkString(paths, conf.SELECT_PARENT_PATH_BATCH)
		for _, p := range pathsArr {
			tmpCond := append(cond, "isdir=1")
			tmpCond = append(tmpCond, fmt.Sprintf(`parent_path in (%s)`, strings.Join(p, ",")))
			start := 0
			for {
				dbRes, err := f.fetchFiles(start, batchSize, tmpCond, "parent_path_index")
				if err != nil {
					f.Ctx.L.Warn("fetchFile err[%v]", err)
					return fsIdDeque, dirMap, err
				}
				if len(dbRes) < 1 {
					break
				}
				fsIdPairs, err := f.insertDir(uid, dbRes, false, isRecycled)
				if err != nil {
					f.Ctx.L.Warn("[insertDir fail] [err: %v]", err)
					return fsIdDeque, dirMap, err
				}
				for _, f := range dbRes {
					dirMap[f.FsId] = f
					deque.AddTail(f.Path)
				}
				for _, pair := range fsIdPairs {
					fsIdDeque.AddTail(pair)
				}
				condAffecteNum += len(dbRes)
				start += batchSize
				if len(dbRes) < batchSize {
					break
				}
			}
		}
	}
	return fsIdDeque, dirMap, nil
}

func (f *FileDelet) insertDir(uid uint64, files []*dao.FileMeta, topFileMeta, isRecycled bool) ([]*FsIdPair, error) {
	operId := f.currentServerMetime - f.currentServerMetime%conf.FILE_PROTECT_DAY
	/* if !isRecycled && topFileMeta && !f.hasRecordOperid {
		f.hasRecordOperid = true
		if !(&redis.Cache{Ctx: f.Ctx}).RecordOperid(f.Uid, operId) {
			return nil, errors.New(conf.ERROR_FILE_DELETE_FAILED)
		}
	} */

	var metaMapArr []map[string]interface{}
	var fsIdPairs []*FsIdPair
	for _, meta := range files {
		metaMap := (&File{}).MetaToMap(meta)
		fsId := (&File{Ctx: f.Ctx}).GenFsIDEx(uid, meta.Path)
		fsIdPairs = append(fsIdPairs, &FsIdPair{fromFsId: meta.FsId, toFsId: fsId})
		metaMap["fs_id"] = fsId
		metaMap["delete_fs_id"] = fsId
		metaMap["extent_int3"] = f.OperaId
		metaMap["share"] = 0
		metaMap["extent_int6"] = meta.FsId

		if !isRecycled {
			if !utils.UseDbTimeForServerMTime() {
				metaMap["server_mtime"] = int(f.currentServerMetime)
			} else {
				metaMap["server_mtime"] = `unix_timestamp()`
			}
			metaMap["real_server_mtime"] = f.currentServerMetime
		}
		if f.DevType == "mac" {
			metaMap["extent_tinyint2"] = 1
		}
		if f.DevType == "pc" {
			metaMap["delete_type"] = 1
		}
		if !isRecycled && f.Op != conf.DEL_PERMANENT_OP {
			if topFileMeta {
				metaMap["isdelete"] = conf.PCS_FILE_RECYCLED_TOP
				metaMap["extent_tinyint5"] = conf.PCS_FILE_PROTECT_TOP
				metaMap["protect_operid"] = f.protectOperId
			} else {
				metaMap["isdelete"] = conf.PCS_FILE_RECYCLED
				metaMap["extent_tinyint5"] = conf.PCS_FILE_PROTECT
			}
		} else {
			metaMap["isdelete"] = conf.PCS_FILE_RECYCLED
			if f.Op == conf.DEL_PERMANENT_OP || f.Type == conf.DEL_RECYCLE_FILE_TYPE || f.Type == conf.DEL_HIDDEN_FILE_TYPE {
				metaMap["isdelete"] = conf.PCS_FILE_DELETED
			}
			if meta.ProtectOperid > int(f.currentServerMetime) {
				metaMap["protect_operid"] = f.currentServerMetime
			}
			if topFileMeta {
				metaMap["extent_tinyint5"] = conf.PCS_FILE_PROTECT_TOP
			} else {
				metaMap["extent_tinyint5"] = conf.PCS_FILE_PROTECT
			}
		}
		metaMapArr = append(metaMapArr, metaMap)
	}

	metaMapChunkArr := utils.ArrayChunkMap(metaMapArr, conf.RUN_SQL_SIZE)
	for _, metaMaps := range metaMapChunkArr {
		_, err := dao.NewFileMetaView(f.Ctx).InsertFileMetas(metaMaps, true)
		if err != nil {
			f.Ctx.L.Warn("[insert file meta fail] [err: %v]", err)
			return nil, err
		}
	}
	if topFileMeta && !isRecycled {
		(&redis.Cache{Ctx: f.Ctx}).RecordProtectCount(f.Uid, operId, len(files))
	}
	return fsIdPairs, nil
}

func (f *FileDelet) process(cond []string, extraTarget []*dao.FileMeta, recordOperate bool) (int, error) {
	f.Ctx.StatusStart()
	defer f.Ctx.StatusEnd()
	condAffecteNum := 0
	memberId := (&File{Ctx: f.Ctx}).GetMemberIdFromPath(f.rawPath)
	updateToManyFile := false
	batchSize := 2000
	for {
		dbRes, err := f.fetchFiles(0, batchSize, cond, "")
		if err != nil {
			f.Ctx.L.Warn("fetchFile err[%v]", err)
			return condAffecteNum, err
		}
		target := dbRes
		if err = f.batchDelete(target, false, memberId); err != nil {
			f.Ctx.L.Warn("batchDelete err[%v]", err)
			return condAffecteNum, err
		}
		condAffecteNum += len(dbRes)
		f.affectRow += len(target)
		if f.affectRow > 100000 && !updateToManyFile {
			f.Ctx.L.Warn("update to many file num[%d]", f.affectRow)
			updateToManyFile = true
		}
		if recordOperate && len(dbRes) != 0 {
			var msgMetaList []map[string]interface{}
			for _, file := range dbRes {
				msgMetaList = append(msgMetaList, f.genSubDelMeta(file))
			}
			errSend := f.MsgSender(msgMetaList, nil)
			if errSend != nil {
				f.Ctx.L.Warn("sendMsg err[%v]", errSend)
			}
		}
		if len(dbRes) < batchSize {
			break
		}
	}
	if len(extraTarget) != 0 {
		if err := f.batchDelete(extraTarget, true, memberId); err != nil {
			f.Ctx.L.Warn("batchDelete err[%v]", err)
			return condAffecteNum, err
		}
		f.affectRow += len(extraTarget)
	}
	return condAffecteNum, nil
}

func (f *FileDelet) cleanRecycleBin() error {
	// 并发锁
	ok := (&file_lock.FileLock{Ctx: f.Ctx}).TryLock(strconv.FormatUint(f.Uid, 10), "", file_lock.LOCK_TYPE_WRITE, file_lock.LOCK_LEVEL_RECYCLE, 3)
	if !ok {
		f.Ctx.L.Warn("[filelock fail] [lock for clean recycle fail] [uid: %d]", f.Uid)
		return errors.New(conf.ERROR_ASYNC_LOCK_EXIST)
	}
	defer func() {
		(&file_lock.FileLock{Ctx: f.Ctx}).UnLock(strconv.FormatUint(f.Uid, 10), "", file_lock.LOCK_TYPE_WRITE, file_lock.LOCK_LEVEL_RECYCLE)
		f.Ctx.L.Info("[unlock for clean recycle] [uid: %d]", f.Uid)
	}()
	f.Ctx.L.Info("[filelock success]")

	appRoot := ""
	appInfo, err := (&App{f.Ctx}).GetAppInfo(f.Appid)
	if err != nil {
		return err
	}
	if appInfo != nil {
		appRoot = appInfo.Dirname
		pathDealMap, errDeal := (&File{Ctx: f.Ctx}).DealPath(f.Uid, appRoot, false)
		if errDeal != nil {
			return errDeal
		}
		appRoot = pathDealMap["path"].(string)
	}
	var cond []string
	cond = append(cond, fmt.Sprintf("user_id=%d", f.Uid))
	deleteCond := "((" + utils.WhereDeletedSql(conf.PCS_FILE_RECYCLED_TOP) + ") or (" + utils.WhereDeletedSql(conf.PCS_FILE_RECYCLED) + "))"
	if f.ProductType == conf.PCS_WORKSPACE_PRODUCT {
		deleteCond = "((" + utils.WhereHiddenDeletedSql(conf.PCS_FILE_RECYCLED_TOP) + ") or (" + utils.WhereHiddenDeletedSql(conf.PCS_FILE_RECYCLED) + "))"
	}
	cond = append(cond, deleteCond)
	if appRoot != "" {
		likePath := utils.Strtr(appRoot, map[string]string{`%`: `\%`, `_`: `\_`})
		pathCond := (`(parent_path like '` + likePath + `/%' or parent_path = '` + appRoot + `')`)
		cond = append(cond, pathCond)
	}

	if f.NeedUpdateProgress {
		num, err1 := f.getFileNum(cond)
		if err1 != nil {
			f.Ctx.L.Warn("[get file num fail] [err: %s]", err1)
		} else {
			f.Ctx.Session[conf.ALREADY_DB_SUM_COUNT] = num
		}
	}

	// 记录用户操作
	ids := (&FileOperate{Ctx: f.Ctx}).InitDelOperLogs([]FileOperateLog{{Uid: f.Uid}}, OP_TYPE_CLEARRECYCLE)

	for i := 0; i < conf.Pcs.FILE_OPER_RETRY; i++ {
		_, err = f.process(cond, []*dao.FileMeta{}, true)
		if err == nil {
			break
		}
		f.Ctx.L.Warn("[clean recycle fail] [retry: %v] [err: %s]", i+1, err)
	}
	if err != nil {
		f.Ctx.L.Warn("delete fail err[%v]", err)
		return err
	}

	// 修改用户操作记录状态为成功
	(&FileOperate{Ctx: f.Ctx}).UpdateOperLogToSuccess(f.Uid, ids)
	if f.NeedUpdateProgress {
		UpdateProgress(100, f.Ctx)
	}
	return nil
}

func (f *FileDelet) deleteShareFile() error {
	topFileMeta, shareDir, err := f.fetchShareDir()
	if err != nil && err.Error() == conf.ERROR_FILE_NOT_EXIST &&
		f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
		f.Ctx.L.Info("[file not exist skip] [ShareDir: %v]", f.ShareDir)
		f.Ctx.Session[conf.FAIL_LIST] = []map[string]interface{}{}
		return nil
	}
	if err != nil {
		f.Ctx.L.Warn("fetch shareDir err[%v]", err)
		return err
	}

	// 并发锁
	// lock path
	ok1 := (&file_lock.FileLock{Ctx: f.Ctx}).TryLock(strconv.FormatUint(topFileMeta.UserId, 10), topFileMeta.Path, file_lock.LOCK_TYPE_WRITE, file_lock.LOCK_LEVEL_PATH, 3)
	if !ok1 {
		f.Ctx.L.Warn("[filelock fail] [lock for delete share file fail] [path: %s]", topFileMeta.Path)
		return errors.New(conf.ERROR_ASYNC_LOCK_EXIST)
	}
	defer func() {
		(&file_lock.FileLock{Ctx: f.Ctx}).UnLock(strconv.FormatUint(topFileMeta.UserId, 10), topFileMeta.Path, file_lock.LOCK_TYPE_WRITE, file_lock.LOCK_LEVEL_PATH)
		f.Ctx.L.Info("[unlock for delete share file] [path: %s]", topFileMeta.Path)
	}()
	// lock recycle
	ok2 := (&file_lock.FileLock{Ctx: f.Ctx}).TryLock(strconv.FormatUint(topFileMeta.UserId, 10), "", file_lock.LOCK_TYPE_READ, file_lock.LOCK_LEVEL_RECYCLE, 3)
	if !ok2 {
		f.Ctx.L.Warn("[filelock fail] [lock recycle for delete share file fail] [uid: %d]", topFileMeta.UserId)
		return errors.New(conf.ERROR_ASYNC_LOCK_EXIST)
	}
	defer func() {
		(&file_lock.FileLock{Ctx: f.Ctx}).UnLock(strconv.FormatUint(topFileMeta.UserId, 10), "", file_lock.LOCK_TYPE_READ, file_lock.LOCK_LEVEL_RECYCLE)
		f.Ctx.L.Info("[unlock recycle for delete share file] [uid: %d]", topFileMeta.UserId)
	}()
	f.Ctx.L.Info("[filelock success]")

	// 记录用户操作
	ids := (&FileOperate{Ctx: f.Ctx}).InitDelOperLogs([]FileOperateLog{{Uid: topFileMeta.UserId, From: topFileMeta.Path}}, OP_TYPE_SHARE)

	(&File{Ctx: f.Ctx}).UpdateTopFolderTime(topFileMeta.UserId, topFileMeta.Path)

	f.rawPath = f.ShareDir
	var shareDirPath []string
	for index, _ := range shareDir {
		shareDirPath = append(shareDirPath, shareDir[index].Path)
	}
	for index, _ := range shareDir {
		f.topFid = append(f.topFid, shareDir[index].FsId)
	}

	if f.NeedUpdateProgress {
		totalNum := 0
		for _, dir := range shareDir {
			path := utils.Addslashes(dir.Path)
			likePath := utils.Strtr(dir.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
			cond := []string{
				fmt.Sprintf("user_id=%d", f.Uid),
				utils.WhereDeletedSql(conf.PCS_FILE_NORMAL),
				fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path),
			}
			num, err1 := f.getFileNum(cond)
			if err1 != nil {
				f.Ctx.L.Warn("[get file num fail] [err: %s]", err1)
			} else {
				totalNum = totalNum + num + 1
			}
		}
		f.Ctx.Session[conf.ALREADY_DB_SUM_COUNT] = totalNum
	}

	for _, dir := range shareDir {
		path := utils.Addslashes(dir.Path)
		likePath := utils.Strtr(dir.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
		cond := []string{
			fmt.Sprintf("user_id=%d", f.Uid),
			utils.WhereDeletedSql(conf.PCS_FILE_NORMAL),
			fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path),
		}

		// 记录用户操作
		(&IslandFile{Ctx: f.Ctx}).RecordUserOperWithoutError(f.Uid, dir.Path)

		if _, errProcess := f.process(cond, []*dao.FileMeta{dir}, false); errProcess != nil {
			f.Ctx.L.Warn("delete fail err[%v]", errProcess)
			return errProcess
		}
	}

	subFileCond, err := f.genShareSubFileCond()
	if err != nil {
		f.Ctx.L.Warn("genShareSubFileCond err[%v]", err)
		return err
	}
	batchSize := 2000
	for {
		dbRes, errFetch := f.fetchFiles(0, batchSize, subFileCond, "")
		if errFetch != nil {
			return errFetch
		}
		for _, item := range dbRes {
			f.topFid = append(f.topFid, item.FsId)
		}
		if _, errProcess := f.process([]string{}, dbRes, false); errProcess != nil {
			f.Ctx.L.Warn("process err[%v]", errProcess)
			return errProcess
		}
		if len(dbRes) < batchSize {
			break
		}
	}
	// 修改用户操作记录状态为成功
	(&FileOperate{Ctx: f.Ctx}).UpdateOperLogToSuccess(topFileMeta.UserId, ids)
	return nil
}

func (f *FileDelet) formFailList() []map[string]interface{} {
	res := []map[string]interface{}{}
	for _, item := range f.Paths {
		newItem := map[string]interface{}{}
		for k, v := range item {
			newItem[k] = v
		}
		newItem["error_code"] = utils.AtoiWithoutError(conf.ERROR_FILE_DELETE_FAILED)
		res = append(res, newItem)
	}
	return res
}

func (f *FileDelet) startFileMulti() (fail []map[string]interface{}, err error) {
	f.quotaAll = 0
	f.quotaHidden = 0
	f.quotaSamsung = 0
	type fileMetaT struct {
		*dao.FileMeta
		SubNum int
	}
	fileMetaArr := []*fileMetaT{}

	if f.isRedo == false {
		f.delRecords = &deleteRecords{}
		f.delRecords.CurrentServerMetime = time.Now().Unix()
	}
	localhost, _ := utils.Gethostname()
	f.delRecords.Worker = append(f.delRecords.Worker, localhost)
	f.fidProcesssed = map[int64]bool{}
	f.topFid = []int64{}
	f.permanentDelete = 1
	f.hasRecordOperid = false
	rawPath := ""
	f.currentServerMetime = f.delRecords.CurrentServerMetime
	// duration, _ := (&unamecache.Unamecache{Ctx: f.Ctx}).GetUserRecycleDuration(f.Uid)
	duration := (&unamecache.Unamecache{Ctx: f.Ctx}).DefaultUserRecycleDuration()
	f.Ctx.L.PushNotice("recycle_time", strconv.Itoa(duration))
	f.protectOperId = f.delRecords.CurrentServerMetime + int64(duration)
	f.Ctx.Session[conf.FAIL_LIST] = f.formFailList()

	progress := 0.0
	step := 100.0
	if len(f.Paths) != 0 {
		step = 1.0 / float64(len(f.Paths)) * 100.0
	}

	//清空回收站
	if f.Type == conf.DEL_RECYCLE_FILE_TYPE {
		if errClean := f.cleanRecycleBin(); errClean != nil {
			f.Ctx.L.Warn("clean recycle err[%v]", errClean)
			return nil, errClean
		}
		return nil, nil
	}

	//删除共享文件夹
	if f.IsDeleteShareDir {
		if errDelShare := f.deleteShareFile(); errDelShare != nil {
			f.Ctx.L.Warn("delete shareDir err[%v]", errDelShare)
			return nil, errDelShare
		}
		return nil, nil
	}

	revisionMap := make(map[int64]int64)
	pathMap := make(map[int64]map[string]interface{})
	//删除隐藏文件 回收站文件 正常文件
	//var needUpdateTopFile bool
	opType := OP_TYPE_PERMANENT
	if f.Op == conf.DEL_PERMANENT_OP {
		f.permanentDelete = 1
		if f.isRedo == false {
			for _, item := range f.Paths {
				var fileItem *dao.FileMeta
				if _, pathExist := item["path"]; pathExist {
					rawPath = item["path"].(string)
					for _, deleteType := range []int{conf.PCS_FILE_NORMAL, conf.PCS_FILE_RECYCLED_TOP, conf.PCS_FILE_RECYCLED} {
						var file *dao.FileMeta
						var errFetch error
						if file, errFetch = f.fetchOneFileMetaByPath(rawPath, deleteType); errFetch != nil {
							if errFetch.Error() == errors.New(conf.ERROR_FILE_NOT_EXIST).Error() {
								continue
							}
							f.Ctx.L.Warn("GetFileMetaOption fail err[%v]", errFetch)
							return []map[string]interface{}{item}, errFetch
						}
						fileItem = file
						break
					}
					if fileItem == nil && f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
						f.Ctx.L.Info("[file not exist skip] [path: %v]", rawPath)
						continue
					}
					if fileItem == nil {
						f.Ctx.L.Warn("file not found path[%s]", rawPath)
						return []map[string]interface{}{item}, errors.New(conf.ERROR_FILE_NOT_EXIST)
					}
				} else {
					fsid := item["fs_id"].(int64)
					fileItem, err = f.fetchOneFileMetaByFsId(fsid, conf.PCS_FILE_MOST)
					if err != nil && err.Error() == conf.ERROR_FILE_NOT_EXIST &&
						f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
						f.Ctx.L.Info("[file not exist skip] [fsid: %v]", fsid)
						continue
					}
					if err != nil {
						f.Ctx.L.Warn("fetchOneFileMetaByFsId[%d] fail errno[%v]", fsid, err)
						return []map[string]interface{}{item}, err
					}
					if parts := strings.Split(fileItem.Path, ":"); len(parts) > 1 {
						rawPath = parts[1]
					} else {
						f.Ctx.L.Warn("path[%s] invalid", fileItem.Path)
						return []map[string]interface{}{item}, errors.New(conf.ERROR_FILE_DELETE_FAILED)
					}
				}

				var cond []string       //按照隐藏文件、正常文件、回收站文件不同场景拼接子文件的where条件
				var condNoPath []string //按照隐藏文件、正常文件、回收站文件不同场景拼接子文件的where条件,无path条件
				if (&File{Ctx: f.Ctx}).FileIsHiddenByFileMeta(fileItem) {
					if fileItem.IsDir != 0 {
						path := utils.Addslashes(fileItem.Path)
						likePath := utils.Strtr(fileItem.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
						cond = append(cond, fmt.Sprintf("user_id=%d", f.Uid))
						cond = append(cond, utils.WhereDeletedSql(conf.PCS_FILE_HIDDEN))
						cond = append(cond, fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path))

						condNoPath = append(condNoPath, fmt.Sprintf("user_id=%d", f.Uid))
						condNoPath = append(condNoPath, utils.WhereDeletedSql(conf.PCS_FILE_HIDDEN))
					}
				} else if fileItem.IsDelete == conf.PCS_FILE_NORMAL {
					if fileItem.IsDir != 0 {
						path := utils.Addslashes(fileItem.Path)
						likePath := utils.Strtr(fileItem.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
						cond = append(cond, fmt.Sprintf("user_id=%d", f.Uid))
						cond = append(cond, utils.WhereDeletedSql(conf.PCS_FILE_NORMAL))
						cond = append(cond, fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path))

						condNoPath = append(condNoPath, fmt.Sprintf("user_id=%d", f.Uid))
						condNoPath = append(condNoPath, utils.WhereDeletedSql(conf.PCS_FILE_NORMAL))
					}
				} else if fileItem.IsDelete == conf.PCS_FILE_RECYCLED || fileItem.IsDelete == conf.PCS_FILE_RECYCLED_TOP {
					if fileItem.IsDir != 0 {
						path := utils.Addslashes(fileItem.Path)
						likePath := utils.Strtr(fileItem.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
						cond = append(cond, fmt.Sprintf("user_id=%d", f.Uid))
						cond = append(cond, fmt.Sprintf("isdelete in (%d,%d)", conf.PCS_FILE_RECYCLED, conf.PCS_FILE_RECYCLED_TOP))
						cond = append(cond, fmt.Sprintf("(extent_tinyint5=0 or extent_tinyint5=%d or extent_tinyint5=%d)", conf.PCS_FILE_PROTECT_TOP, conf.PCS_FILE_PROTECT))
						cond = append(cond, fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path))
						if utils.UseRealServerMTimeForSearch() {
							cond = append(cond, fmt.Sprintf("real_server_mtime = %d", fileItem.RealServerMtime))
							condNoPath = append(condNoPath, fmt.Sprintf("real_server_mtime = %d", fileItem.RealServerMtime))
						} else {
							cond = append(cond, fmt.Sprintf("server_mtime = %d", fileItem.ServerMtime))
							condNoPath = append(condNoPath, fmt.Sprintf("server_mtime = %d", fileItem.ServerMtime))
						}

						condNoPath = append(condNoPath, fmt.Sprintf("user_id=%d", f.Uid))
						condNoPath = append(condNoPath, fmt.Sprintf("isdelete in (%d,%d)", conf.PCS_FILE_RECYCLED, conf.PCS_FILE_RECYCLED_TOP))
						condNoPath = append(condNoPath, fmt.Sprintf("(extent_tinyint5=0 or extent_tinyint5=%d or extent_tinyint5=%d)", conf.PCS_FILE_PROTECT_TOP, conf.PCS_FILE_PROTECT))
					}
				}
				f.delRecords.Records = append(f.delRecords.Records, deleteRecord{TopFileMeta: *fileItem, BottomFileMetaCond: cond, BottomFileMetaCondNoPath: condNoPath})
			}
		}
	} else {
		//needUpdateTopFile = true
		opType = OP_TYPE_RECYCLE
		f.permanentDelete = 0
		if f.isRedo == false {
			for _, item := range f.Paths {
				var fileItem *dao.FileMeta
				isDelete := conf.PCS_FILE_NORMAL
				if f.Type == conf.DEL_HIDDEN_FILE_TYPE {
					isDelete = conf.PCS_FILE_HIDDEN
				}
				if _, pathExist := item["path"]; pathExist {
					rawPath = item["path"].(string)
					fileItem, err = f.fetchOneFileMetaByPath(rawPath, isDelete)
					if err != nil && err.Error() == conf.ERROR_FILE_NOT_EXIST &&
						f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
						f.Ctx.L.Info("[file not exist skip] [path: %v]", item["path"])
						continue
					}
					if err != nil {
						return []map[string]interface{}{item}, err
					}
				} else {
					fsid := item["fs_id"].(int64)
					fileItem, err = f.fetchOneFileMetaByFsId(fsid, isDelete)
					if err != nil && err.Error() == conf.ERROR_FILE_NOT_EXIST &&
						f.SkipNotExist == conf.FILE_NOT_EXIST_SKIP {
						f.Ctx.L.Info("[file not exist skip] [fsid: %v]", fsid)
						continue
					}
					if err != nil {
						return []map[string]interface{}{item}, err
					}
					if parts := strings.Split(fileItem.Path, ":"); len(parts) > 1 {
						rawPath = parts[1]
					} else {
						f.Ctx.L.Warn("path[%s] invalid", fileItem.Path)
						return []map[string]interface{}{item}, errors.New(conf.ERROR_FILE_DELETE_FAILED)
					}
				}
				revisionMap[fileItem.FsId], _ = item["base_revision"].(int64)
				pathMap[fileItem.FsId] = item
				var cond []string
				var condNoPath []string
				if fileItem.IsDir != 0 {
					path := utils.Addslashes(fileItem.Path)
					likePath := utils.Strtr(fileItem.Path, map[string]string{`%`: `\%`, `_`: `\_`}) + "/%"
					cond = append(cond, fmt.Sprintf("user_id=%d", f.Uid))
					cond = append(cond, utils.WhereDeletedSql(isDelete))
					cond = append(cond, fmt.Sprintf("(parent_path like \"%s\" or parent_path=\"%s\")", likePath, path))

					condNoPath = append(condNoPath, fmt.Sprintf("user_id=%d", f.Uid))
					condNoPath = append(condNoPath, utils.WhereDeletedSql(isDelete))
				}
				f.delRecords.Records = append(f.delRecords.Records, deleteRecord{TopFileMeta: *fileItem, BottomFileMetaCond: cond, BottomFileMetaCondNoPath: condNoPath})
			}
		}
	}

	// 防止一个请求中对相同目录或父子目录进行删除
	// path相同 => 保留一个, 父子目录 => 保留父目录
	delPath := make(map[string]int)
	for i, record := range f.delRecords.Records {
		path := record.TopFileMeta.Path

		var tmp []string
		for p, _ := range delPath {
			tmp = append(tmp, p)
		}
		for _, p := range tmp {
			if utils.IsSubPathOrSame(p, path) {
				path = ""
				break
			}
			if utils.IsSubPathOrSame(path, p) {
				delete(delPath, p)
			}
		}
		if path != "" {
			delPath[path] = i
		}
	}
	if len(delPath) != len(f.delRecords.Records) {
		var newDelRecord []deleteRecord
		for index, record := range f.delRecords.Records {
			if i, ok := delPath[record.TopFileMeta.Path]; ok && index == i {
				newDelRecord = append(newDelRecord, record)
			}
		}
		f.delRecords.Records = newDelRecord
	}
	f.Ctx.L.Debug("[delRecords: %+v]", f.delRecords.Records)

	lockPaths := make(map[string]bool)
	for index, r := range f.delRecords.Records {
		f.topFid = append(f.topFid, f.delRecords.Records[index].TopFileMeta.FsId)
		lockPaths[file_lock.LockPath(strconv.FormatUint(f.Uid, 10), r.TopFileMeta.Path)] = false
	}

	// 并发锁
	// lock path
	defer func() {
		for path, locked := range lockPaths {
			if !locked {
				continue
			}
			(&file_lock.FileLock{Ctx: f.Ctx}).UnLock(strconv.FormatUint(f.Uid, 10), path, file_lock.LOCK_TYPE_WRITE, file_lock.LOCK_LEVEL_PATH)
			f.Ctx.L.Info("[unlock for delete] [path: %s]", path)
		}
	}()

	for path, _ := range lockPaths {
		// lock
		ok := (&file_lock.FileLock{Ctx: f.Ctx}).TryLock(strconv.FormatUint(f.Uid, 10), path, file_lock.LOCK_TYPE_WRITE, file_lock.LOCK_LEVEL_PATH, 3)
		if !ok {
			f.Ctx.L.Warn("[filelock fail] [path: %s]", path)
			return []map[string]interface{}{}, errors.New(conf.ERROR_ASYNC_LOCK_EXIST)
		}
		lockPaths[path] = true
	}
	// lock recycle
	ok := (&file_lock.FileLock{Ctx: f.Ctx}).TryLock(strconv.FormatUint(f.Ctx.UserId, 10), "", file_lock.LOCK_TYPE_READ, file_lock.LOCK_LEVEL_RECYCLE, 3)
	if !ok {
		f.Ctx.L.Warn("[filelock fail] [lock recycle for delete fail] [uid: %d]", f.Ctx.UserId)
		return []map[string]interface{}{}, errors.New(conf.ERROR_ASYNC_LOCK_EXIST)
	}
	defer func() {
		(&file_lock.FileLock{Ctx: f.Ctx}).UnLock(strconv.FormatUint(f.Ctx.UserId, 10), "", file_lock.LOCK_TYPE_READ, file_lock.LOCK_LEVEL_RECYCLE)
		f.Ctx.L.Info("[unlock recycle for delete] [uid: %d]", f.Ctx.UserId)
	}()
	f.Ctx.L.Info("[filelock success]")

	// skipnotexist参数会导致不存在目录被跳过,f.Path与f.delRecords.Records不一致,需要修改FAIL_LIST
	failList := f.Ctx.Session[conf.FAIL_LIST].([]map[string]interface{})
	if len(failList) > 0 {
		var newFailList []map[string]interface{}
		for _, r := range f.delRecords.Records {
			path := strings.Replace(r.TopFileMeta.Path, fmt.Sprintf("%d:/", r.TopFileMeta.UserId), "", 1)
			path = utils.Trim(path, "/")
			for _, failMap := range failList {
				if failPath, ok := failMap["path"].(string); ok && utils.Trim(failPath, "/") == path {
					newFailList = append(newFailList, failMap)
					break
				}
				if failUid, ok := failMap["fs_id"].(int64); ok && failUid == r.TopFileMeta.FsId {
					newFailList = append(newFailList, failMap)
					break
				}
			}
		}
		f.Ctx.Session[conf.FAIL_LIST] = newFailList
	}
	if f.EnableRedo {
		errrec := cacheDeleteRecords(f.Taskid, f.delRecords, f.Ctx)
		if errrec != nil {
			f.Ctx.L.Warn("cacheDeleteRecords fail")
			return []map[string]interface{}{}, errors.New(conf.ERROR_FILE_DELETE_FAILED)
		}
	}

	if f.NeedUpdateProgress {
		totalNum := 0
		for _, record := range f.delRecords.Records {
			totalNum++
			if len(record.BottomFileMetaCond) < 1 {
				continue
			}
			num, err1 := f.getFileNum(record.BottomFileMetaCond)
			if err1 != nil {
				f.Ctx.L.Warn("[get file num fail] [err: %s]", err1)
			} else {
				totalNum += num
			}
		}
		f.Ctx.Session[conf.ALREADY_DB_SUM_COUNT] = totalNum
	}

	for index, _ := range f.delRecords.Records {
		subNum := 0
		cond := f.delRecords.Records[index].BottomFileMetaCond
		condNoPath := f.delRecords.Records[index].BottomFileMetaCondNoPath
		fileItem := &f.delRecords.Records[index].TopFileMeta
		baseRevision, item, errCheck := f.checkBaseRevision(f.Paths, strings.TrimLeft(fileItem.Path, fmt.Sprintf("%d:", f.Uid)))
		if errCheck != nil {
			return []map[string]interface{}{item}, errCheck
		}

		// 记录用户操作
		(&IslandFile{Ctx: f.Ctx}).RecordUserOperWithoutError(f.Uid, fileItem.Path)
		delType := opType
		if opType == OP_TYPE_PERMANENT && (fileItem.IsDelete == -3 || fileItem.IsDelete == -1) {
			delType = OP_TYPE_FROMCYCLE
		}
		if f.OndupType == conf.PCS_ONDUP_RECYCLE {
			delType = OP_TYPE_ONDUP_RECYCLE
		}
		if f.OndupType == conf.PCS_ONDUP_OVERWRITE {
			delType = OP_TYPE_ONDUP_OVERWRITE
		}
		ids := (&FileOperate{Ctx: f.Ctx}).InitDelOperLogs([]FileOperateLog{{Uid: f.Uid, From: fileItem.Path}}, delType)

		if opType == OP_TYPE_RECYCLE && f.ApiName == conf.Delete {
			(&File{Ctx: f.Ctx}).UpdateTopFolderTime(f.Uid, fileItem.Path)
		}

		st := time.Now().UnixNano()
		subNum, err = f.hierarchyProcess(cond, condNoPath, fileItem)
		/*
			以下为非层级操作逻辑
			sub := 0
			for i := 0; i < conf.Pcs.FILE_OPER_RETRY; i++ {
				sub, err = f.process(cond, []*dao.FileMeta{fileItem}, false)
				subNum += sub
				if err == nil {
					break
				}
				f.Ctx.L.Warn("[delete fail] [retry: %v] [err: %s]", i+1, err)
			} */
		f.Ctx.L.Info("[uid:%v] [IsHierarchy:true] [affect:%v] [cost:%vns]", f.Uid, subNum, time.Now().UnixNano()-st)
		if err != nil {
			f.Ctx.L.Warn("delete fail err[%v]", err)
			item := map[string]interface{}{}
			if index < len(f.Paths) {
				item = f.Paths[index]
			}
			return []map[string]interface{}{item}, err
		}
		// 修改用户操作记录状态为成功
		(&FileOperate{Ctx: f.Ctx}).UpdateOperLogToSuccess(f.Uid, ids)
		fileMetaArr = append(fileMetaArr, &fileMetaT{FileMeta: fileItem, SubNum: subNum})
		succItem := map[string]interface{}{}
		failList := f.Ctx.Session[conf.FAIL_LIST].([]map[string]interface{})
		if len(failList) > 0 {
			for k, v := range failList[0] {
				if k != "error_code" {
					succItem[k] = v
				}
			}
			if baseRevision != 0 {
				succItem["base_revision"] = strconv.FormatUint(baseRevision, 10)
			}
			succItem["size"] = strconv.FormatUint(fileItem.Size, 10)
			succItem["real_server_mtime"] = strconv.Itoa(fileItem.RealServerMtime)
			succItem["server_mtime"] = strconv.Itoa(fileItem.ServerMtime)
			succItem["isdir"] = strconv.Itoa(fileItem.IsDir)
		} else {
			f.Ctx.L.Warn("index[%d] larger than len(Path)[%d]", index, len(f.Paths))
		}
		succAllList := f.Ctx.Session[conf.SUCC_ALL_LIST].([]map[string]interface{})
		succAllList = append(succAllList, succItem)
		f.Ctx.Session[conf.SUCC_ALL_LIST] = succAllList
		if len(failList) > 0 {
			failList = failList[1:]
		}
		f.Ctx.Session[conf.FAIL_LIST] = failList
		if len(succAllList) <= conf.MAX_NUM_SUCCLIST_STORE {
			f.Ctx.Session[conf.SUCC_LIST] = succAllList
		} else {
			f.Ctx.Session[conf.SUCC_LIST] = succAllList[:conf.MAX_NUM_SUCCLIST_STORE]
		}
		progress += step
		if f.NeedUpdateProgress {
			UpdateProgress(int(progress), f.Ctx)
		}
	}

	if len(fileMetaArr) != 0 {
		mainMsg := []map[string]interface{}{}
		for index, _ := range fileMetaArr {
			msgIns := (&Message{Ctx: f.Ctx})
			meta := msgIns.GenBasicMeta(fileMetaArr[index].FileMeta)
			meta["permanent"] = f.permanentDelete
			meta["status"] = fileMetaArr[index].Status
			meta["sub_count"] = fileMetaArr[index].SubNum
			meta["isdelete"] = fileMetaArr[index].IsDelete
			mainMsg = append(mainMsg, meta)
		}
		errSend := f.MsgSender(mainMsg, nil)
		if errSend != nil {
			f.Ctx.L.Warn("sendMsg err[%v]", errSend)
		}
	}

	return nil, nil
}

func (f *FileDelet) FileMulti() (fail []map[string]interface{}, err error) {
	//检查是否作弊
	if err = f.CheckMalicious(0); err != nil {
		return nil, err
	}
	if f.EnableRedo {
		errRec, rec := getDeleteRecords(f.Taskid, f.Ctx)
		if errRec != nil {
			f.Ctx.L.Warn("getDelRec fail")
			return nil, errors.New(conf.ERROR_FILE_DELETE_FAILED)
		}
		f.delRecords = rec
		f.isRedo = rec != nil
	}
	if f.lockForDelete() == false {
		f.Ctx.L.Warn("lock file fail")
		return fail, errors.New(conf.ERROR_ASYNC_LOCK_EXIST)
	}
	defer func() {
		f.unlockForDelete()
	}()

	f.Ctx.Session[conf.SUCC_ALL_LIST] = []map[string]interface{}{}
	fail, err = f.startFileMulti()
	f.Ctx.L.PushNotice("quotaAffect", fmt.Sprintf("%d-%d-%d", f.quotaAll, f.quotaSamsung, f.quotaHidden))
	f.Ctx.Session[conf.PCS_EXTRA].(map[string]interface{})["affect_files"] = f.affectRow
	f.Ctx.Session[conf.ALREADY_TARGET_SIZE] = f.affectRow
	//保证删除超限制那次也返回成功
	f.CheckMaliciousWithoutError(int(f.quotaAll + f.quotaSamsung + f.quotaHidden))
	f.AfterDealDone(err)
	return fail, err
}

func (f *FileDelet) CheckMalicious(quotaNum int) error {
	var (
		total int64
		err   error
		limit = conf.MAX_DELETE_SIZE
	)
	if !conf.OPEN_DELETE_LIMIT {
		return nil
	}
	//仅检查作弊时去动态检查quota
	if quotaNum == 0 {
		total, err = (&quota.Quota{f.Ctx}).GetUserTotalQuota(f.Uid, "", 0, f.Appid)
		if err != nil {
			f.Ctx.L.Warn("maliciouscheck quota fail.")
		}
		if total > conf.MAX_DELETE_SIZE/2 {
			limit = int(2 * total)
		}
	}
	//如果单日删除量超过(低于25T用户限制50T，高于25T用户限制2*用户total)，禁止删除
	if size, err := IncrDeleteSize(f.Ctx, f.Uid, quotaNum); err == nil && size > limit {
		f.Ctx.L.Warn("[malicious user by delete] [uid:%d] [limit:%d]", f.Uid, limit)
		return errors.New(conf.ERROR_NETWORK_ERROR)
	} else if err != nil {
		if err.Error() == "ERR value is not an integer or out of range" {
			f.Ctx.L.Warn("[malicious user over redis max value by delete] [uid:%d] err[%s] [limit:%d]", f.Uid, err, limit)
			return errors.New(conf.ERROR_NETWORK_ERROR)
		}
		//不是超过最大值的其他err放过
		f.Ctx.L.Warn("[redis err] [uid:%d] err[%v] [limit:%d]", f.Uid, err, limit)
	}
	return nil
}

func (f *FileDelet) CheckMaliciousWithoutError(quotaNum int) {
	err := f.CheckMalicious(quotaNum)
	if err == nil {
		return
	}
}
