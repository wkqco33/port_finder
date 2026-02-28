BINARY  := port_finder
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X port_finder/cmd.Version=$(VERSION)"

INSTALL_DIR := $(HOME)/.local/bin

.PHONY: all build clean install uninstall test

all: build

## build: 바이너리 빌드 (./port_finder)
build:
	go build $(LDFLAGS) -o $(BINARY) .

## clean: 빌드 결과물 삭제
clean:
	rm -f $(BINARY)

## install: ~/.local/bin 에 설치
install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "설치 완료: $(INSTALL_DIR)/$(BINARY)"

## uninstall: ~/.local/bin 에서 제거
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "제거 완료: $(INSTALL_DIR)/$(BINARY)"

## test: 유닛 테스트 실행
test:
	go test ./... -v

## help: 사용 가능한 명령 목록 출력
help:
	@grep -E '^## ' Makefile | sed 's/## /  make /'
