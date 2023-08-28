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
	if strings.Contains(pkg, module_name) {
		// fmt.Println("这里是匹配的的 pkh", pkg)
		return pkg
	}

	return ""

}

func GetRecv(node *ast.FuncDecl) string {
	var recName string
	if node.Recv != nil {
		if node.Recv.List != nil && len(node.Recv.List) > 0 {
			// recName = node.Recv.List[0].Type.(*ast.StarExpr).X.(*ast.Ident).Name
			if ty, ok := node.Recv.List[0].Type.(*ast.StarExpr); ok {
				if tx, ok := ty.X.(*ast.Ident); ok {
					recName = tx.Name
				}

			}

		}
	}

	return recName
}
func GetFilePackage(node *ast.File) string {
	if node.Name != nil {
		// fmt.Println("当前所处的 package--->", node.Name.Name)
	}
	// fmt.Println(node)

	return node.Name.Name
}
