package asttool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

type CallerSignle struct {
	Package string // 包名
	Func    string // 函数名
	IsOOP   bool   // 是否为OOP函数
	Method  string // 方法名
	Reciver string // 接收器
}
type OOP struct {
	PackageName string
	Reciver     string
}

// 右定义
var methMap = make(map[*ast.Object]OOP, 0)

// 左定义
var methMapRigh = make(map[*ast.Object]*ast.Object, 0)

// var  methMap  make(map[*ast.Object]OOP{},0)
func GetFuncCaller(node ast.Node, fset *token.FileSet, containMap map[string][]CallerSignle, calc int, test int, codePath string, findMap map[string]bool) {

	switch v := node.(type) {
	case *ast.AssignStmt:
		id, right, who, call := GetAssignStmt(v)
		// 如果是左有，右没有  目前默认为 oop 对象的初始化
		if id != nil && right == nil && who != "" {
			methMap[id] = OOP{
				PackageName: who,
				Reciver:     call,
			}
			methMap[right] = OOP{
				PackageName: who,
				Reciver:     call,
			}
		}
		//  如果左有 .无论如何都加入
		if id != nil && right != nil {
			methMapRigh[id] = right
		}

		for {
			//  左
			v, ok := methMapRigh[id]
			if !ok {
				break
			}
			d, ok := methMap[v]
			if ok {
				methMap[id] = OOP{
					PackageName: d.PackageName,
					Reciver:     d.Reciver,
				}
				break
			}
			right = v

		}

		// // 如果是左有，会引发一次溯源 ，
		// if right != nil {
		// 	if v, ok := methMap[right]; ok {
		// 		methMap[right] = OOP{
		// 			PackageName: v.PackageName,
		// 			Reciver:     v.Reciver,
		// 		}
		// 		// fmt.Println("定义 ", v.PackageName, v.Reciver)
		// 	}
		// }
		// if id != nil && right != nil {
		// 	methMap[id] = OOP{
		// 		PackageName: methMap[right].PackageName,
		// 		Reciver:     methMap[right].Reciver,
		// 	}
		// }

	case *ast.CallExpr:
		//   new caller  这里仅保存  包名+函数名的方式
		obj, who, calll := GetCaller(v)
		if obj == nil {
			if findMap[who] {
				fmt.Printf("调用的包名 %s ， 调用的函数 : %s \n", who, calll)
			}
		} else {
			if v, ok := methMap[obj]; ok {
				fmt.Println("通过对象查找 ---> ", calll, v.PackageName, v.Reciver)

			}

		}
		//  方法 定位器

	}
}

// 返回值是 map  key 是文件名  value 是  CallerSignle  数组，保存了函数相关的信息
func SignleCallerq(file []string) (containMap map[string][]CallerSignle) {
	containMap = make(map[string][]CallerSignle, 0)
	// importMap := make(map[string]map[CallerImport]bool, 0)
	calc := 0
	all := 0
	for _, codePath := range file {

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, codePath, nil, 0)

		if err != nil {
			continue
		}
		ast.Inspect(f, func(node ast.Node) bool {
			//  文件级别扫描，扫描文件所在的包，函数，方法和接收器
			_, findMap := SignleImport(file, "baidu/netdisk/pcs-go-pcsapi")

			GetFuncCaller(node, fset, containMap, calc, all, codePath, findMap)
			return true
		})
	}
	return containMap
}

// 包含有目标函数的部分解
func SignleCallerAimeFunc(file []string, funcName string) (containMap map[string][]CallerSignle) {
	containMap = make(map[string][]CallerSignle, 0)
	// importMap := make(map[string]map[CallerImport]bool, 0)
	calc := 0
	all := 0
	for _, codePath := range file {

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, codePath, nil, 0)

		if err != nil {
			continue
		}

		// 遍历顶层声明列表
		for _, decl := range f.Decls {
			// 查找目标函数
			if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Name.Name == funcName {
				// 输出函数的节点信息
				// ast.Print(fset, funcDecl)
				ast.Inspect(funcDecl, func(node ast.Node) bool {
					//  文件级别扫描，扫描文件所在的包，函数，方法和接收器
					GetFuncCaller(node, fset, containMap, calc, all, codePath, nil)
					return true
				})
				break
			}
		}

	}
	return containMap
}

func GetAssignStmt(node *ast.AssignStmt) (id *ast.Object, right *ast.Object, where string, reciver string) {
	assignStmt := node

	// 查找 := 左边的标识符
	if len(assignStmt.Lhs) == 0 {
		return
	}
	ident, ok := assignStmt.Lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	id = ident.Obj
	expr := assignStmt.Rhs[0]
	exprc := assignStmt.Rhs[0]
	if unaryExpr, ok := expr.(*ast.UnaryExpr); ok {
		expr = unaryExpr.X
	} else {

		//  暂时无用
		callExpr1, ok := exprc.(*ast.CallExpr)
		if !ok {
			return id, right, where, reciver
		}
		sc, ok := callExpr1.Fun.(*ast.SelectorExpr)
		if !ok {

			return id, right, where, reciver
		}
		i, ok := sc.X.(*ast.Ident)
		if !ok {
			return id, right, where, reciver
		}
		right = i.Obj

	}
	// 查找 := 右边的复合字面量
	compositeLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return
	}

	// 查找复合字面量的类型
	selectorExpr, ok := compositeLit.Type.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if o, ok := selectorExpr.X.(*ast.Ident); ok && right == nil {
		right = o.Obj
	}
	where = GetAstIdent(selectorExpr.X.(*ast.Ident))
	reciver = selectorExpr.Sel.Name

	return id, right, where, reciver
}
