.PHONY: all build clean target

BUILD_DIR = build

all: build

build:
	@echo "==> Configuring and building with CMake..."
	@cmake -B $(BUILD_DIR)
	@cmake --build $(BUILD_DIR)

clean:
	@echo "==> Cleaning build directory..."
	@rm -rf $(BUILD_DIR)

install: build
	@echo "==> Installing port_finder to system..."
	@sudo cmake --install $(BUILD_DIR)

uninstall:
	@echo "==> Uninstalling port_finder from system..."
	@if [ -f $(BUILD_DIR)/install_manifest.txt ]; then \
		sudo xargs rm -f < $(BUILD_DIR)/install_manifest.txt; \
	else \
		echo "설치 기록(install_manifest.txt)을 찾을 수 없습니다. (make install을 먼저 실행했는지 확인해주세요.)" >&2; \
	fi

rebuild: clean build
