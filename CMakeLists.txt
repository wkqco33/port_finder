cmake_minimum_required(VERSION 3.10)

project(port_finder VERSION 0.1.0 LANGUAGES C)

# C Standard: C17
set(CMAKE_C_STANDARD 17)
set(CMAKE_C_STANDARD_REQUIRED ON)

add_compile_options(-Wall -Wextra -Wpedantic)

# Executable target - src/port_info.c 추가
add_executable(port_finder 
    src/main.c 
    src/port_info.c
    src/log.c
)

# Include directories
target_include_directories(port_finder PRIVATE include)

# Install rules
install(TARGETS port_finder DESTINATION bin)
