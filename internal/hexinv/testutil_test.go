package hexinv

import "os"

// osWriteFile 抽一层便于测试里写临时文件。
func osWriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
