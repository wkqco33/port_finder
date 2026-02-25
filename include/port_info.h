#ifndef PORT_INFO_H
#define PORT_INFO_H

#include <stdbool.h>
#include <stdint.h>
#include <sys/types.h>

#define PORT_FINDER_NAME "PORT_FINDER"
#define PORT_FINDER_VERSION "0.1.0"

/**
 * @brief 포트를 사용하는 프로세스 정보를 담는 구조체
 */
typedef struct {
  uint16_t port;
  pid_t pid;
  char process_name[256];
} port_process_info;

/**
 * @brief 특정 포트를 사용 중인 프로세스를 찾아 정보를 반환합니다.
 * @param port 검색할 포트 번호
 * @param info 결과를 담을 구조체 포인터
 * @return 성공 시 true, 찾지 못했을 시 false
 */
bool find_process_by_port(uint16_t port, port_process_info *info);

/**
 * @brief 특정 PID를 사용 중인 프로세스를 종료합니다.
 * @param pid 종료할 프로세스의 PID
 */
void kill_process_by_pid(pid_t pid);

#endif // PORT_INFO_H
