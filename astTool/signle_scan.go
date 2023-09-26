package asttool

import (
	"fmt"
	"go/ast"
	"go/token"
)

type CallerScan struct {
	Package string // 包名
	Func    string // 函数名
	IsOOP   bool   // 是否为OOP函数
	Method  string // 方法名
}
type CallerImportSacn struct {
	Rename string //重命名的包名
	Origin string // 基于path 的 包名，仅保存源码的包
	Splite string // 去除路径的b包名，方便快速索引
}

//  单文件的扫描器，用来扫描对应文件的
// 1： import 了哪些包
//  2： 对应的函数所 call 哪些函数和方法

// 根据node返回该节点的起止行，如果有
func getFuncCallerScan(node ast.Node, fset *token.FileSet) CallerScan {
	switch v := node.(type) {
	case *ast.CallExpr:
		if v != nil {

		}
		callExpr, _ := node.(*ast.CallExpr)
		if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			who, calll := scanSelectExpr(selExpr)

			obj := GetFuncDefine(selExpr)
			var who1 string
			if obj != nil {
				// fmt.Println("---> 需要溯源，")

				who1, _ = ScanObj(obj)
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
			// if who == "" && who1 == "" {
			// 	calc = calc + 1
			// }

			// test = test + 1
			var st CallerScan
			if (who != "" || who1 != "") && calll != "" {
				st.Package = who
				st.Method = who1
				st.Func = calll

			}
			return st

			// // 提取函数名的定义部分

		}
	case *ast.SelectorExpr:
	default:

	}
	return CallerScan{}

}
func scanSelectExpr(node *ast.SelectorExpr) (string, string) {
	var from string
	var call string
	call = GetAstIdent(node.Sel)
	if id, ok := node.X.(*ast.Ident); ok {
		from = GetAstIdent(id)
		if id.Obj != nil {
			from = id.Obj.Name
		}
	}
	//   解决三级调用  ctx.L.Warn
	// from = GetThreeCall(node)
	tmp := GetThreeCall(node)
	if tmp != "" {
		from = tmp
	}
	// if ppx, ok := node.X.(*ast.SelectorExpr); ok {
	// 	if id, ok := ppx.X.(*ast.Ident); ok {
	// 		from = GetAstIdent(id)
	// 		if id.Obj != nil {
	// 			from = id.Obj.Name
	// 		}
	// 	}
	// }
	//   解决在定义期间调用  举例：  parentFileRevisionMeta, errGet := (&service.FileRevision{Ctx: ctx}).GetRevisionDetail(req.UserId, req.Path, req.ParentRevision)
	tmp = GetDefineCall(node)
	if tmp != "" {
		from = tmp
	}

	if node.Sel == nil {
		return "", ""
	}

	call = GetAstIdent(node.Sel)

	return from, call

}

// 只解决简单的包名+ 函数名的解析
func GetCaller(node *ast.CallExpr) (obj *ast.Object, who string, calll string) {
	if node.Fun != nil { // 判断是否为函数
		if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
			// who, calll = scanSelectExpr(selExpr)
			if x, ok := selExpr.X.(*ast.Ident); ok {
				who = x.Name
				if x.Obj != nil {
					obj = x.Obj
				}
			}
			if selExpr.Sel != nil {
				calll = selExpr.Sel.Name
			}

		}
	}
	if who != "" && calll != "" {
		return
	} else {
		return
	}

}
