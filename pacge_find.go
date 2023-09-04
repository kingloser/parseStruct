package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"parseStruct/astTool"
	"parseStruct/util"
	// "strings"
)

// A  -->   <---  B  --->   <--- c
type NodeStruct struct {
	Node interface{} //  node 指针

	Next    interface{} // 指向下一个影响的指针
	UpNode  interface{} // 上一个node
	KeyInfo string      // 记录该节点的所包含的核心信息
	Extra   string      // 备用

}
type FuncInfo struct {
	FuncName string // 函数名
	Reciver  string // 函数接器
}

// 定义一个二维slice用来存储没有覆盖的行和列
var section [][]int
var calc float32
var test float32

// 15 - 22  是一个switch  语句
const startLine = 33
const endLine = 34

var codePath []string
var ptrNode interface{}

//  设计 两个数据结构
//  1： map  来单纯存储 第一个 影响的参数，并标记搜索的深度
//  双向链表 ---》用来存储其拓扑关系

var NodeMap map[any]int // 将抓取到的 node 指针作为 key ，把扩散深度作为 value

func main() {
	calc = 0
	test = 0
	// codePathTmp := "test/baidu/netdisk/pcs-go-pcsapi/models/service/file_delete.go"
	codePathTmp := "test/baidu/netdisk/pcs-go-pcsapi/action/file/copy.go"
	section = append(section, []int{0, 1199})
	astAnaly(codePathTmp)

}

var moduleName = "baidu/netdisk/pcs-go-pcsapi"
var ff *token.FileSet

// 执行ast  分析
func astAnaly(codePath string) {
	file := util.ScanProject("test/baidu/netdisk/pcs-go-pcsapi")
	// file := util.ScanProject("test/baidu/netdisk/pcs-go-pcsapi/action/file/copy.go")
	// fmt.Println("*****")
	// 解析源文件
	// funcScan := make(map[string]map[string])
	fileMap := make(map[string][]string, 0)
	// strFileMap := make(map[string][]FuncInfo, 0)
	strFileMap := make(map[string]map[FuncInfo]bool, 0)

	for _, codePath := range file {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, codePath, nil, 0)

		if err != nil {
			continue
		}
		var pac string
		ast.Inspect(f, func(node ast.Node) bool {
			//  进入一定会先 get package
			switch v := node.(type) {
			case *ast.File:
				pac = astTool.GetFilePackage(v)
			}
			getLine(node, fset)
			findKeyNode(node, fset, fileMap, pac, strFileMap)
			getFuncCaller(node, fset)

			return true
		})
	}
	fmt.Println("-->解概率 析出的%v", calc/test)
	//   strFileMap  KEY 是包名，value map map中 key是函数名 + 接收器
	// for key, value := range strFileMap {
	// 	fmt.Printf("所在的包 %v ，函数名包含", key)
	// 	// fmt.Println("所在的包，%v，包含的 import %v", key, value)
	// 	for k, _ := range value {
	// 		fmt.Printf(" 函数 %v ， 接收器 : %s ", k.FuncName, k.Reciver)
	// 	}
	// 	fmt.Println("\n")
	// }

}

// 根据node返回该节点的起止行，如果有
func getLine(node ast.Node, fset *token.FileSet) (error, int, int) {
	if node != nil {
		posIsValid := node.Pos().IsValid()
		if posIsValid == true {
			//获取其实和结束行
			startLine := fset.Position(node.End()).Line
			endLine := fset.Position(node.Pos()).Line
			return nil, startLine, endLine
		}
		return errors.New("-1"), -1, -1
	}
	return errors.New("-1"), -1, -1
}

func getFuncCaller(node ast.Node, fset *token.FileSet) {
	switch v := node.(type) {
	case *ast.CallExpr:
		if v != nil {

		}
		callExpr, _ := node.(*ast.CallExpr)
		if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			// // 提取方法和函数名称
			// // fmt.Println(selExpr.Sel.Name)
			// who, calll := scanSelectExpr(selExpr)

			// obj := getFuncDefine(selExpr)
			// var who1 string
			// if obj != nil {
			// 	who1 , _  = scanObj(obj)
			// }
			// fmt.Printf("调用的对象 %s ， 调用的函数 : %s   溯源-->%s \n  ", who, calll, who1)

			who, calll := scanSelectExpr(selExpr)

			obj := getFuncDefine(selExpr)
			var who1 string
			if obj != nil {
				// fmt.Println("---> 需要溯源，")
				who1, _ = scanObj(obj)
				// if  obj.Decl!=nil{

				// }

				if who1 != "" && obj.Decl != nil {
					fmt.Printf("调用的对象 %s ， 调用的方法 : %s   溯源-->%s \n  ", who, calll, who1)
				}
				// else {
				// 	//  暂时存在一个调用 const类型的，暂时先忽略掉，后续搜索的时候直接抛弃掉即可
				// 	fmt.Printf(" 溯源失败： 调用的对象 %s ， 调用的函数 : %s \n", who, calll)

				// }
			} else {
				fmt.Printf("调用的包名 %s ， 调用的函数 : %s \n", who, calll)

			}
			if who == "" && who1 == "" {
				calc = calc + 1
			}
			test = test + 1
			// // 提取函数名的定义部分

		}
	case *ast.SelectorExpr:
		// who, calll := scanSelectExpr(v)

		// obj := getFuncDefine(v)
		// var who1 string
		// if obj != nil {
		// 	// fmt.Println("---> 需要溯源，")
		// 	who1, _ = scanObj(obj)
		// 	// if  obj.Decl!=nil{

		// 	// }

		// 	if who1 != "" && obj.Decl != nil {
		// 		fmt.Printf("调用的对象 %s ， 调用的方法 : %s   溯源-->%s \n  ", who, calll, who1)
		// 	}
		// 	// else {
		// 	// 	//  暂时存在一个调用 const类型的，暂时先忽略掉，后续搜索的时候直接抛弃掉即可
		// 	// 	fmt.Printf(" 溯源失败： 调用的对象 %s ， 调用的函数 : %s \n", who, calll)

		// 	// }
		// } else {
		// 	fmt.Printf("调用的包名 %s ， 调用的函数 : %s \n", who, calll)

		// }

	}

}

func scanSelectExpr(node *ast.SelectorExpr) (string, string) {
	var from string
	var call string
	call = getAstIdent(node.Sel)
	if id, ok := node.X.(*ast.Ident); ok {
		from = getAstIdent(id)
		if id.Obj != nil {
			from = id.Obj.Name
		}
	}
	//   解决三级调用  ctx.L.Warn
	// from = getThreeCall(node)
	tmp := getThreeCall(node)
	if tmp != "" {
		from = tmp
	}
	// if ppx, ok := node.X.(*ast.SelectorExpr); ok {
	// 	if id, ok := ppx.X.(*ast.Ident); ok {
	// 		from = getAstIdent(id)
	// 		if id.Obj != nil {
	// 			from = id.Obj.Name
	// 		}
	// 	}
	// }
	//   解决在定义期间调用  举例：  parentFileRevisionMeta, errGet := (&service.FileRevision{Ctx: ctx}).GetRevisionDetail(req.UserId, req.Path, req.ParentRevision)
	tmp = getDefineCall(node)
	if tmp != "" {
		from = tmp
	}
	// if ppy, ok := node.X.(*ast.ParenExpr); ok {
	// 	if ppz, ok := ppy.X.(*ast.UnaryExpr); ok {
	// 		if ppx, ok := ppz.X.(*ast.CompositeLit); ok {
	// 			if ppx.Type != nil {
	// 				if id, ok := ppx.Type.(*ast.SelectorExpr); ok {
	// 					if i, ok := id.X.(*ast.Ident); ok {
	// 						from = getAstIdent(i)
	// 					}
	// 					// from = getAstIdent(id)
	// 					// from = id.Obj.Name
	// 				}
	// 			}

	// 		}
	// 	}

	// }

	if node.Sel == nil {
		return "", ""
	}

	call = getAstIdent(node.Sel)

	return from, call

}

// 解决在定义期间调用  举例：  parentFileRevisionMeta, errGet := (&service.FileRevision{Ctx: ctx}).GetRevisionDetail(req.UserId, req.Path, req.ParentRevision)
func getDefineCall(node *ast.SelectorExpr) string {
	// 解决三级调用 ctx.L.Warn
	from := ""
	parent, ok := node.X.(*ast.ParenExpr)
	if !ok {
		return from
	}
	unary, ok := parent.X.(*ast.UnaryExpr)
	if !ok {
		return from
	}
	composite, ok := unary.X.(*ast.CompositeLit)
	if !ok || composite.Type == nil {
		return from
	}

	if id, ok := composite.Type.(*ast.SelectorExpr); ok {
		if i, ok := id.X.(*ast.Ident); ok {
			from = getAstIdent(i)
		}
	}

	return from
}
func getThreeCall(node *ast.SelectorExpr) string {
	//   解决三级调用  ctx.L.Warn
	var from string
	if ppx, ok := node.X.(*ast.SelectorExpr); ok {
		if id, ok := ppx.X.(*ast.Ident); ok {
			if id.Obj != nil {
				from = getObjName(id.Obj)
			}
		}
	}
	return from
}

func getObjName(node *ast.Object) string {
	if node != nil {
		return node.Name
	}
	return ""
}

//	func  getXExpr(node *ast.Ident) string{
//		if  node.Obj!= nil{
//			return node.
//		}
//	}
func getFuncDefine(node *ast.SelectorExpr) *ast.Object {
	if node.X == nil {
		return nil
	}
	if ident, ok := node.X.(*ast.Ident); ok {

		if ident.Obj != nil {
			// scanObj(ident.Obj)
			return ident.Obj
		}
	}
	return nil

}

// 溯源，去查找，对象的定义的来源，其实就是制订了 对象来源于哪里，通过那个包来定义
func scanObj(node *ast.Object) (packagesName string, funName string) {
	//  node.Decl.(*ast.AssignStmt).Rhs[0].X.Type.
	// node.Decl.(*ast.AssignStmt).Rhs[0].X.Type.(*ast.SelectorExpr).X.Name
	// node.Decl.(*ast.AssignStmt).Rhs[0].X.Type.(*ast.SelectorExpr).Sel.Name
	assignStmt, ok := node.Decl.(*ast.AssignStmt)
	if !ok {
		return "", ""
	}

	// 判断AssignStmt是否为nil
	if assignStmt.Rhs == nil || len(assignStmt.Rhs) == 0 {
		return "", ""
	}

	unaryExpr, ok := assignStmt.Rhs[0].(*ast.UnaryExpr)
	if !ok {
		return "", ""
	}

	// 判断UnaryExpr是否为nil
	if unaryExpr.X == nil {
		return "", ""
	}

	compositeLit, ok := unaryExpr.X.(*ast.CompositeLit)
	if !ok {
		return "", ""
	}

	// 判断CompositeLit是否为nil
	if compositeLit.Type == nil {
		return "", ""
	}

	selectorExpr, ok := compositeLit.Type.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}

	// 判断SelectorExpr是否为nil
	if selectorExpr.X == nil {
		return "", ""
	}

	ident, ok := selectorExpr.X.(*ast.Ident)
	if !ok {
		return "", ""
	}

	if ident.Name != "" {
		packagesName = ident.Name
	}
	// if ident.Sel != nil {
	// 	funName = ident.Sel.Name
	// }
	if selectorExpr.Sel != nil {
		funName = selectorExpr.Sel.Name
	}
	fmt.Println("-->", packagesName, funName)
	if packagesName == "" || packagesName == " " {
		fmt.Println("----> 这里为空")
	}
	// fmt.Println(node.Decl.(*ast.AssignStmt).Rhs[0].(*ast.UnaryExpr).X.(*ast.CompositeLit).Type.(*ast.SelectorExpr).X.(*ast.Ident).Name)
	return
}
func getAstIdent(node *ast.Ident) string {
	if node != nil {
		return node.Name
	}
	return ""
}
func findKeyNode(node ast.Node, fset *token.FileSet, file map[string][]string, pac string, strFile map[string]map[FuncInfo]bool) *ast.AssignStmt {

	switch v := node.(type) {

	// case *ast.File:
	// 	pac = astTool.GetFilePackage(v)
	// _, ok := file[pac]
	// if !ok {
	// 	file[pac] = []string{""}
	// }
	case *ast.FuncDecl:
		if v.Name.Name != "" {
			_, ok := file[pac]
			fName := v.Name.Name
			if !ok {
				file[pac] = []string{v.Name.Name}
				tmp := make(map[FuncInfo]bool, 0)
				tmpF := FuncInfo{
					FuncName: v.Name.Name,
				}
				tmpF.Reciver = astTool.GetRecv(v)
				tmp[tmpF] = true

				strFile[pac] = tmp
			} else {
				if v.Name.Name != "" {
					file[pac] = append(file[pac], fName)
					// strFile[pac] = append(strFile[pac], FuncInfo{FuncName: v.Name.Name})
					// tmp := make(map[FuncInfo]bool, 0)
					tmpF := FuncInfo{
						FuncName: v.Name.Name,
					}
					tmpF.Reciver = astTool.GetRecv(v)
					// tmp[tmpF] = true
					strFile[pac][tmpF] = true
					// strFile[pac] =

				}

			}
		}

	case *ast.ImportSpec:
		fuc := astTool.GetPackageImport(v, moduleName)
		_, ok := file[pac]
		if !ok {
			// file[pac] = []string{fuc}
		} else {
			if fuc != "" {
				// file[pac] = append(file[pac], fuc)
			}

		}

	}
	return nil
}

//	新增 大区域的标记，如果bigflage为ture,则标记认为大范围，例如 10-20行的代码，在没有覆盖
//
// 的11-12行，也是
func sectionBig(sec [][]int, rLine int, leLine int, bigFlage bool) (bool, int) {
	for j, v := range sec {
		if len(v) != 2 {
			log.Printf("区间长度异常，不予处理")
			return false, -1
		}
		//  严格模式，主要过滤处严格在起止行列中的node信息
		if bigFlage == false {
			if rLine >= v[1] && leLine <= v[0] {
				return true, j
			}
		} else {
			if (v[0] >= leLine && v[0] <= rLine) || (rLine >= v[1] && leLine <= v[1]) {
				return true, j
			}
		}

	}
	return false, -1
}

// PathExists 判断一个文件或文件夹是否存在 ,避免解析不存在的文件出现panic
func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// node.Name.Obj.Decl.Type.Params.List   获取函数定义的入参
//
//	获取函数的入参
func FuncNodeParse(node *ast.FuncDecl) map[interface{}]bool {
	tmpObj := make(map[interface{}]bool)
	if name := node.Name; name != nil {
		if obj := name.Obj; obj != nil {
			if decl := obj.Decl; decl != nil {
				if ntype := decl.(*ast.FuncDecl).Type; ntype != nil {
					if params := ntype.Params; params != nil {
						if paramsList := params.List; paramsList != nil {
							for index, value := range paramsList {
								fmt.Println(index)
								fmt.Println(value)
								tmp := ExperParse(value)
								if tmp != nil {
									tmpObj[tmp] = true
									fmt.Println("---> 抓取到内部的 obj")
								}

							}
						}
					}
				}

			}
		}
	}
	return tmpObj
}

// 解析ast.field ，并返回其中的指针
func ExperParse(node *ast.Field) interface{} {
	if node != nil {
		if name := node.Names; name != nil {
			//  暂时取第一个，后续有样本集再去看为什么
			if obj := name[0].Obj; obj != nil {
				return obj
			}
		}
	}
	return nil
}
