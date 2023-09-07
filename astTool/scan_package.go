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

// GetPackageImportSignel 函数根据传入的ast.ImportSpec类型的node节点和module_name字符串，返回对应的导入包名
func GetPackageImportSignel(node *ast.ImportSpec, module_name string) (origin string, rename string) {
	pkg := node.Path.Value
	if node.Name != nil {
		//  如果存在重命名
		rename = fmt.Sprintf("%s", node.Name)
		// return "", fmt.Sprintf("%s", node.Name)
	}
	if strings.Contains(pkg, module_name) {
		// fmt.Println("这里是匹配的的 pkh", pkg)
		// return pkg, ""
		origin = pkg
		return  origin, rename
	}
	
	return  origin, rename
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

// 溯源，去查找，对象的定义的来源，其实就是制订了 对象来源于哪里，通过那个包来定义
func ScanObj(node *ast.Object) (packagesName string, funName string) {
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

// func(ctx *utils.Context)  解决。如果是 hi 传参的定义的查找
func ScanObjDefine(node *ast.Object) (packagesName string) {
	assignStmt, ok := node.Decl.(*ast.Field)
	if !ok {
		return
	}
	if assignStmt.Type == nil {
		return
	}
	selce, ok := assignStmt.Type.(*ast.StarExpr)
	if !ok {
		return
	}
	sx, ok := selce.X.(*ast.SelectorExpr)
	if !ok {
		return
	}
	sd, ok := sx.X.(*ast.Ident)
	if !ok {
		return
	}

	packagesName = GetAstIdent(sd)
	return

}

// 解决在定义期间调用  举例：  parentFileRevisionMeta, errGet := (&service.FileRevision{Ctx: ctx}).GetRevisionDetail(req.UserId, req.Path, req.ParentRevision)
func GetDefineCall(node *ast.SelectorExpr) string {
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
			from = GetAstIdent(i)
		}
	}

	return from
}
func GetThreeCall(node *ast.SelectorExpr) string {
	//   解决三级调用  ctx.L.Warn
	var from string
	if ppx, ok := node.X.(*ast.SelectorExpr); ok {
		if id, ok := ppx.X.(*ast.Ident); ok {
			if id.Obj != nil {
				from = GetObjName(id.Obj)
				//  尝试查找定义
				ul := ScanObjDefine(id.Obj)
				if ul != "" {
					from = ul
					fmt.Println("查找define定义--->", ul)

				}
			}
		}
	}
	return from
}

// GetObjName 返回给定节点对象的名称
func GetObjName(node *ast.Object) string {
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
//
// GetFuncDefine 函数接收一个指向ast.SelectorExpr的指针作为参数，返回一个指向ast.Object的指针
func GetFuncDefine(node *ast.SelectorExpr) *ast.Object {
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

// GetAstIdent 返回一个ast.Ident类型的指针所指向的标识符的名称
func GetAstIdent(node *ast.Ident) string {
	if node != nil {
		return node.Name
	}
	return ""
}
