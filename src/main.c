#include "log.h"
#include "port_info.h"
#include <errno.h>
#include <getopt.h>
#include <stdio.h>
#include <stdlib.h>

static void print_usage(const char *prog_name) {
  printf("Usage: %s [options]\n", prog_name);
  printf("\n포트 번호를 검색하여 해당 포트를 사용하는 프로세스를 찾는 "
         "프로그램입니다.\n");
  printf("예시: %s -p 8080\n\n", prog_name);
  printf("Basic options:\n");
  printf("  -h, --help    도움말 출력\n");
  printf("  -p, --port    검색할 포트 번호 (1-65535)\n");
  printf("  -v, --version 프로그램 버전 출력\n");
}

int main(int argc, char **argv) {
  long port_number = 0;
  int opt;
  static const struct option long_options[] = {
      {"help", no_argument, 0, 'h'},
      {"port", required_argument, 0, 'p'},
      {"version", no_argument, 0, 'v'},
      {0, 0, 0, 0}};

  while ((opt = getopt_long(argc, argv, "hp:v", long_options, NULL)) != -1) {
    switch (opt) {
    case 'p': {
      char *endptr;
      errno = 0;
      port_number = strtol(optarg, &endptr, 10);
      if (errno != 0 || *endptr != '\0') {
        port_number = -1; // 유효하지 않은 포트 값이면 아래 if에서 걸러짐
      }
      break;
    }
    case 'h':
      print_usage(argv[0]);
      return EXIT_SUCCESS;
    case 'v':
      printf("%s version %s\n", PORT_FINDER_NAME, PORT_FINDER_VERSION);
      return EXIT_SUCCESS;
    default:
      print_usage(argv[0]);
      return EXIT_FAILURE;
    }
  }

  if (port_number <= 0 || port_number > 65535) {
    log_error("유효한 포트 번호(1-65535)를 입력해주세요.");
    return EXIT_FAILURE;
  }

  printf("포트 %ld 사용 중인 프로세스 검색 중...\n", port_number);

  port_process_info info = {0};
  bool result = find_process_by_port((uint16_t)port_number, &info);

  if (result) {
    printf("발견! 포트 %ld를 사용 중인 프로세스 정보:\n", port_number);
    printf("    - PID  : %d\n", info.pid);
    printf("    - NAME : %s\n", info.process_name);

    printf("\n[*] 해당 프로세스를 종료하시겠습니까? (y/N): ");
    char answer_str[16];
    if (fgets(answer_str, sizeof(answer_str), stdin) != NULL) {
      if (answer_str[0] == 'y' || answer_str[0] == 'Y') {
        kill_process_by_pid(info.pid);
      } else {
        printf("프로세스 종료를 보류했습니다.\n");
      }
    }
  } else {
    printf("포트 %ld를 사용하는 프로세스를 찾을 수 없습니다.\n", port_number);
  }

  return EXIT_SUCCESS;
}
