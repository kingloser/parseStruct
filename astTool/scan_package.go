package astTool

import (
	"fmt"
	"go/ast"
	"strings"
)

func GetPackageImport(node *ast.ImportSpec, module_name string) string {
	//  如果存在重命名的问题，直接使用重命名
	pkg := node.Path.Value
	// fmt.Println(pkg)
	//  在采集阶段暂时不考虑重命名的行为
	if node.Name != nil {
		// fmt.Println("这里有重命名的 --->", node.Name)
		return fmt.Sprintf("%s", node.Name)
	}
	if  strings.Contains(pkg,module_name){
		// fmt.Println("这里是匹配的的 pkh", pkg)
		return  pkg
	}

	return ""

}

func GetFilePackage(node *ast.File) string {
	if node.Name != nil {
		// fmt.Println("当前所处的 package--->", node.Name.Name)
	}
	// fmt.Println(node)

	return node.Name.Name
}
