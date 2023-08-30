package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// scanProject 函数用于扫描指定路径下的所有非隐藏文件，并去除单测文件
func ScanProject(folderPath string) (proPath []string) {
	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println("遍历文件夹出错:", err)
			return err
		}
		// 忽略隐藏文件和文件夹，以及文件夹本身 ,以及_test.go的单测文件
		if info.Name()[0] == '.' && info.IsDir() || info.Name()[0] == '_' || strings.Contains(info.Name(), "_test.go") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() && strings.Contains(info.Name(), ".go") {
			proPath = append(proPath, path)
		}

		// proPath = path
		return nil
	})

	if err != nil {
		fmt.Println("出错:", err)
	}
	// fmt.Print(proPath)
	return
}
