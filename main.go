package main

import (
	"fmt"
	"parseStruct/asttool"
	"parseStruct/util"
)

func main() {
	// codePathTmp := "test/baidu/netdisk/pcs-go-pcsapi/models/service/file_delete.go"
	// codePathTmp := "test/baidu/netdisk/pcs-go-pcsapi/action/file/copy.go"
	codePathTmp := "test/baidu/netdisk/pcs-go-pcsapi"
	moduleName := "baidu/netdisk/pcs-go-pcsapi"

	// section = append(section, []int{0, 1199})
	file := util.ScanProject(codePathTmp)
	dm := asttool.FullScan(file)
	// for key, value := range dm {
	// 	fmt.Println("包名", key)
	// 	fmt.Println("包路径", value.PackageName)
	// 	for k, v := range value.FuncIndex {
	// 		fmt.Println("函数名", k.FuncName, "接收器：", k.Reciver)
	// 		fmt.Println("函数所在的位置", v)
	// 	}
	// }
	inGo := "test/baidu/netdisk/pcs-go-pcsapi/action/file/copy.go"

	inFile := util.ScanProject(inGo)
	asttool.SignleCallerl(inFile)
	start(inFile, dm, moduleName)

}
func start(file []string, fullContain map[string]asttool.MoudleFull, moduleName string) {
	for _, f := range file {
		next := RecursiveQuery(f, fullContain, moduleName)
		if next == "" {
			return
		} else {
			RecursiveQuery(next, fullContain, moduleName)
		}
	}

}

// 递归查询器 返回出他节点路径
func RecursiveQuery(inFile string, fullContain map[string]asttool.MoudleFull, moduleName string) (aime string) {
	if len(inFile) == 0 {
		return
	}
	file := []string{inFile}
	// Rename string //重命名的包名
	// Origin string // 基于path 的 包名，仅保存源码的包
	// Splite string // 去除路径的b包名，方便快速索引
	importInfo := asttool.SignleImport(file, moduleName)
	for key, value := range importInfo {
		fmt.Println("所在的", key)
		for _ = range value {
			// fmt.Println("重命名的包名%v, 原始包名%v , sk  %v ", value[i].Rename, value[i].Origin, value[i].Splite)
		}

	}
	callFunc, _ := asttool.SignleCallerq(file, "")
	for key, value := range callFunc {
		fmt.Println(" 所在的文件", key)
		for i := range value {
			fmt.Printf("引用所在的包%v,  方法 名%v, 函数名  %v \n", value[i].Package, value[i].Method, value[i].Func)
		}

	}

	//  如果 caller 找到的包 在 full 里， 则 去找对应的 函数，然后再再解析对应的 函数
	//   开始递归
	for _, value := range callFunc {
		for i := range value {
			// fmt.Println("引用所在的包%v, 是否为 oop %v, 方法 名%v, 函数名  %v ", value[i].Package, value[i].IsOOP, value[i].Method, value[i].Func)

			// if     index, ok := fullContain[]
			// if  value[i].Package

			if index, ok := fullContain[value[i].Package]; ok {
				// index.FuncIndex
				tmp := asttool.FuncInfoAll{
					FuncName: value[i].Func,
					// Reciver: value[i].Reciver,
				}

				if _, ok := index.FuncIndex[tmp]; ok {
					fmt.Println("一个所指向的的文件:???", index.PackageName, "需要查找的函数", value[i].Func, "存在的接收器", value[i].Reciver)
				}
				tmp.FuncName = value[i].Method
				if _, ok := index.FuncIndex[tmp]; ok {
					fmt.Println("一个所指向的的文件:", index.PackageName, "需要查找的函数", value[i].Func)
				}

			}

		}
	}
	return ""

}
