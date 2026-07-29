package httpapi

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
)

//go:embed all:dist
var embeddedDist embed.FS

// GetEmbeddedDistFS 返回嵌入的前端打包产物 FS（子目录 dist）。
// 若构建时未执行 make web，该目录为空，server 会提示用户先构建前端。
func GetEmbeddedDistFS() (fs.FS, error) {
	if _, err := fs.Stat(embeddedDist, "dist/index.html"); err != nil {
		return nil, fmt.Errorf("前端 index.html 未嵌入: %w", err)
	}
	sub, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		log.Printf("前端资源未嵌入或路径错误: %v", err)
		return nil, err
	}
	return sub, nil
}
