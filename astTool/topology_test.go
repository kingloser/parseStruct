package asttool

import (
	"fmt"
	"parseStruct/util"
	"testing"
)

func TestFullScan(t *testing.T) {
	codePathTmp := "../test/baidu/netdisk/pcs-go-pcsapi"

	file := util.ScanProject(codePathTmp)
	// dm := FullScan(file)

	tests := []struct {
		name    string
		args    []string
		want    map[string]MoudleFull
		wantErr bool
	}{
		{
			name: "1",
			args: file,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FullScan(file)

			for key, value := range got {
				fmt.Println("包名", key)
				fmt.Println("包路径", value.PackageName)
				for k, v := range value.FuncIndex {
					fmt.Println("函数名", k.FuncName, "接收器：", k.Reciver)
					fmt.Println("函数所在的位置", v)
				}
			}
			tmp := FuncInfoAll{
				FuncName: "Superfile3Commit",
				Reciver:  "Superfile3",
			}
			m := got["superfile3"]
			fmt.Println(m.FuncIndex[tmp])
		})

	}
}
