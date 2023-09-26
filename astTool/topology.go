package asttool

import (
	// "fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	// "path/filepath"
)

// 文件中 函数和解析器
type FuncInfoAll struct {
	FuncName string // 函数名
	Reciver  string // 函数接器
}

type MoudleFull struct {
	// Origin   string // 基于path 的 包名，仅保存源码的包
	PackageName string                 // 去除路径的b包名，方便快速索引
	FuncIndex   map[FuncInfoAll]string // 函数的索引，当知道包内函数是，可以快速定位到改函数所在的源码的位置
}




// 用于扫描整个项目的报名和函数，包含函数/方法所在的文件
func FullScan(file []string) (containMap map[string]MoudleFull) {
	containMap = make(map[string]MoudleFull, 0)
	// importMap := make(map[string]map[CallerImport]bool, 0)

	for _, codePath := range file {

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, codePath, nil, 0)

		if err != nil {
			continue
		}
		var pac string
		ast.Inspect(f, func(node ast.Node) bool {
			//  进入一定会先 get package  如果不是，艹 ，死给你看
			switch v := node.(type) {
			case *ast.File:
				pac = GetFilePackage(v)
			}
			//  文件级别扫描，扫描文件所在的包，函数，方法和接收器
			getFilePackageFunc(node, fset, pac, containMap, codePath)
			return true
		})
	}
	return containMap
}
func getFilePackageFunc(node ast.Node, fset *token.FileSet, pac string, containMap map[string]MoudleFull, codePath string) {
	// containMap = make(map[string]MoudleFull,0)
	switch v := node.(type) {
	case *ast.FuncDecl:
		if v.Name.Name != "" {
			_, ok := containMap[pac]
			//  有没有必须，先这样保持
			if !ok {
				// file[pac] = []string{v.Name.Name}
				//   如果第一map中不存在该包名，则直接放入
				split := filepath.Dir(codePath)

				tmpN := MoudleFull{
					PackageName: split,
					FuncIndex:   make(map[FuncInfoAll]string),
				}
				containMap[pac] = tmpN
				tmp := FuncInfoAll{}
				tmp.FuncName = v.Name.Name //函数名
				// tmp.Reciver = GetRecv(v)
				containMap[pac].FuncIndex[tmp] = codePath
			} else {
				if v.Name.Name != "" {
					tmp := FuncInfoAll{
						FuncName: v.Name.Name,
						// FuncIndex:   make(map[FuncInfoAll]string),
					}
					// tmp.Reciver = GetRecv(v)
					containMap[pac].FuncIndex[tmp] = codePath
				}

			}
		}

	}
	// return  containMap
}
