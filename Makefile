# Go 编译器
GO := go

# 默认目标
all: clean build compress

# 清理目标文件
clean:
	rm -rf build


# 编译目标文件
build:clean deps prepare
	# Linux
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w " -trimpath -o ./build/PowerPing_linux_amd64 main.go
	env GOOS=linux GOARCH=arm64 go build -ldflags="-s -w " -trimpath -o ./build/PowerPing_linux_arm64 main.go
	find build -type f -executable | xargs upx -9

# 安装依赖
deps:
	$(GO) mod download

prepare:
	bash -c "[[ -d build ]] || mkdir build"

.PHONY: all clean build deps compress run