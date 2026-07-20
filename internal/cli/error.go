package cli

import (
	"fmt"
	"os"
)

// HandleError 统一错误处理
func HandleError(err error, message string) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	os.Exit(1)
}

// HandleErrorWithCode 带退出码的错误处理
func HandleErrorWithCode(err error, message string, exitCode int) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	os.Exit(exitCode)
}
