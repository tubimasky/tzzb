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
	account position profit trend build-all web web-embed serve

all: build

## build: 编译到当前目录 ./tzzb（自动构建并嵌入前端）
build: web-embed
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP) .

## install: 安装到 ~/go/bin (或 $(GOPATH)/bin)
install: web-embed
	@mkdir -p ~/go/bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o ~/go/bin/$(APP) .
	@echo "✓ 已安装到 ~/go/bin/$(APP)"
	@echo "确保 ~/go/bin 在你的 PATH 中"

## uninstall: 删除 $(GOPATH)/bin/tzzb
uninstall:
	rm -f "$$(go env GOPATH)/bin/$(APP)"

## clean: 清理本地二进制与交叉编译产物
clean:
	rm -f $(APP) $(APP).exe
	rm -rf dist/
	rm -rf internal/httpapi/dist/*

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

## account / position / profit / trend: 常用命令快捷入口
account: build
	./$(APP) account list

position: build
	./$(APP) position stock

profit: build
	./$(APP) profit summary

trend: build
	./$(APP) trend show

## build-all: 交叉编译到 dist/
build-all: clean web-embed
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
	@echo "  make trend"
	@echo "  make web        # 构建前端"
	@echo "  make serve      # 启动 Web 服务"

## web: 构建前端（打包到 web/dist）
web:
	@echo "→ 构建前端..."
	cd web && npm ci && npm run build
	@echo "✓ 前端已打包到 web/dist"

## web-embed: 构建前端并复制到 internal/httpapi/dist 用于 go:embed
web-embed:
	@echo "→ 构建前端并准备嵌入..."
	@cd web && npm ci && npm run build
	@mkdir -p internal/httpapi/dist
	@rm -rf internal/httpapi/dist/*
	@cp -r web/dist/* internal/httpapi/dist/
	@echo "✓ 前端已嵌入到 internal/httpapi/dist"

## serve: 启动 Web 服务（自动嵌入前端资源）
serve: build
	./$(APP) serve
