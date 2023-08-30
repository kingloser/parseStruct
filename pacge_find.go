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

			return true
		})
	}
	//   strFileMap  KEY 是包名，value map map中 key是函数名 + 接收器
	for key, value := range strFileMap {
		fmt.Printf("所在的包 %v ，函数名包含", key)
		// fmt.Println("所在的包，%v，包含的 import %v", key, value)
		for k, _ := range value {
			fmt.Printf(" 函数 %v ， 接收器 : %s ", k.FuncName, k.Reciver)
		}
		fmt.Println("\n")
	}

	
	
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
