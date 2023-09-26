package asttool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

type CallerImportSingle struct {
	Rename string //重命名的包名
	Origin string // 基于path 的 包名，仅保存源码的包
	Splite string // 去除路径的b包名，方便快速索引
}

// 存储单个文件的所应用的包名，这里为概率行查找铺垫
// map  key 是当前的路径名 ， value  []CallerImportSingle
//  访问实例
// 	im := astTool.SignleImport(file ,moduleName)
// for  key ,value := range im {
// 	fmt.Println("所在的", key)
// 	for i :=range value{
// 		fmt.Println("重命名的包名%v, 原始包名%v , sk  %v ", value[i].Rename,value[i].Origin, value[i].Splite)
// 	}

// }
func ScanSignleImport(node ast.Node, fset *token.FileSet, sigleImport map[string][]CallerImportSingle, codePath string, moduleName string, findMap map[string]bool) {
	switch v := node.(type) {

	case *ast.ImportSpec:
		origin, rename := GetPackageImportSignel(v, moduleName)
		origin = strings.ReplaceAll(origin, "\"","")
		split := filepath.Base(origin)
		var tmp CallerImportSingle
		if rename != "" || origin != "" {
			tmp = CallerImportSingle{
				Rename: rename,
				Origin: origin,
				Splite: split,
			}
			findMap[split] = true
			// tmpN := make(map[CallerImportSingle]bool, 0)
			// tmpN[tmp] = true
			sigleImport[codePath] = append(sigleImport[codePath], tmp)
		}
		// fmt.Println(codePath, "rename:", origin, ", origin:", rename, "去除path 的报名", split)
	}
	return
}
func SignleImport(file []string, moduleName string) (containMap map[string][]CallerImportSingle, findMap map[string]bool) {
	containMap = make(map[string][]CallerImportSingle, 0)
	findMap = make(map[string]bool, 0)
	// importMap := make(map[string]map[CallerImport]bool, 0)

	for _, codePath := range file {

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, codePath, nil, 0)

		if err != nil {
			continue
		}
		ast.Inspect(f, func(node ast.Node) bool {
			//  文件级别扫描，扫描文件所在的包，函数，方法和接收器
			ScanSignleImport(node, fset, containMap, codePath, moduleName, findMap)
			return true
		})
	}
	return containMap, findMap
}
