# 投资账本 CLI
APP     := tzzb
LDFLAGS := -s -w

# 交叉编译目标：make build-all
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

.PHONY: all build install uninstall clean run fmt tidy vet test help \
	account position profit build-all

all: build

## build: 编译到当前目录 ./tzzb
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP) .

## install: 安装到 $(go env GOPATH)/bin
install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

## uninstall: 删除 $(GOPATH)/bin/tzzb
uninstall:
	rm -f "$$(go env GOPATH)/bin/$(APP)"

## clean: 清理本地二进制与交叉编译产物
clean:
	rm -f $(APP) $(APP).exe
	rm -rf dist/

## run: 编译后执行，额外参数用 ARGS=... 传递
## 例: make run ARGS="account list"
run: build
	./$(APP) $(ARGS)

## fmt: gofmt
fmt:
	gofmt -w .

## tidy: go mod tidy
tidy:
	go mod tidy

## vet: go vet
vet:
	go vet ./...

## test: 跑测试（目前可能无测试文件）
test:
	go test ./...

## account / position / profit: 常用命令快捷入口
account: build
	./$(APP) account list

position: build
	./$(APP) position stock

profit: build
	./$(APP) profit summary

## build-all: 交叉编译到 dist/
build-all: clean
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out="dist/$(APP)-$${os}-$${arch}"; \
		[ "$$os" = "windows" ] && out="$$out.exe"; \
		echo "→ $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags "$(LDFLAGS)" -o "$$out" . || exit 1; \
	done
	@ls -lh dist/

## help: 显示目标说明
help:
	@echo "用法: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
	@echo ""
	@echo "示例:"
	@echo "  make build"
	@echo "  make install"
	@echo "  make run ARGS='position stock'"
	@echo "  make account"
