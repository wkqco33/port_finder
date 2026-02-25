#include "port_info.h"
#include "log.h"
#include <dirent.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

/**
 * @brief /proc/net/tcp에서 포트에 해당하는 inode를 찾습니다.
 */
static unsigned long get_inode_for_port(uint16_t port) {
  FILE *fp = fopen("/proc/net/tcp", "r");
  if (!fp) {
    log_error(
        "/proc/net/tcp 파일을 열 수 없습니다: 루트 권한이 필요할 수 있습니다.");
    return 0;
  }

  char line[512];
  unsigned long local_port, inode;

  // 첫 줄(헤더) 건너뜀
  fgets(line, sizeof(line), fp);

  while (fgets(line, sizeof(line), fp)) {
    // 형식: sl local_address remote_address st tx_queue rx_queue tr tm->when
    // retr uid timeout inode ... local_address는 hex_ip:hex_port 형식
    if (sscanf(line, "%*d: %*X:%lX %*X:%*X %*X %*X:%*X %*X:%*X %*X %*d %*d %lu",
               &local_port, &inode) == 2) {
      if ((uint16_t)local_port == port) {
        fclose(fp);
        return inode;
      }
    }
  }

  fclose(fp);
  return 0;
}

/**
 * @brief 모든 PID의 fd를 돌며 특정 inode를 사용하는 프로세스를 찾습니다.
 */
static pid_t find_pid_by_inode(unsigned long target_inode) {
  DIR *dir = opendir("/proc");
  if (!dir) {
    log_warn("/proc 디렉터리를 열 수 없습니다.");
    return -1;
  }

  struct dirent *entry;
  while ((entry = readdir(dir)) != NULL) {
    // PID 폴더인지 확인 (숫자로만 이루어짐)
    if (entry->d_name[0] < '0' || entry->d_name[0] > '9')
      continue;

    pid_t pid = (pid_t)atoi(entry->d_name);
    char fd_path[512];
    snprintf(fd_path, sizeof(fd_path), "/proc/%d/fd", pid);

    DIR *fd_dir = opendir(fd_path);
    if (!fd_dir)
      continue;

    struct dirent *fd_entry;
    while ((fd_entry = readdir(fd_dir)) != NULL) {
      if (fd_entry->d_name[0] == '.')
        continue;

      char link_path[1024];
      char target[1024];
      snprintf(link_path, sizeof(link_path), "%s/%s", fd_path,
               fd_entry->d_name);

      ssize_t len = readlink(link_path, target, sizeof(target) - 1);
      if (len != -1) {
        target[len] = '\0';
        // socket:[inode] 형식 확인
        unsigned long inode;
        if (sscanf(target, "socket:[%lu]", &inode) == 1) {
          if (inode == target_inode) {
            closedir(fd_dir);
            closedir(dir);
            return pid;
          }
        }
      }
    }
    closedir(fd_dir);
  }
  closedir(dir);
  return -1;
}

bool find_process_by_port(uint16_t port, port_process_info *info) {
  unsigned long inode = get_inode_for_port(port);
  if (inode == 0)
    return false;

  pid_t pid = find_pid_by_inode(inode);
  if (pid == -1)
    return false;

  info->port = port;
  info->pid = pid;

  // 프로세스 이름 읽기
  char comm_path[512];
  snprintf(comm_path, sizeof(comm_path), "/proc/%d/comm", pid);
  FILE *fp = fopen(comm_path, "r");
  if (fp) {
    if (fgets(info->process_name, sizeof(info->process_name), fp)) {
      // 개행 문자 제거
      info->process_name[strcspn(info->process_name, "\n")] = 0;
    }
    fclose(fp);
  } else {
    strncpy(info->process_name, "unknown", sizeof(info->process_name));
  }

  return true;
}

/**
 * @brief 특정 PID를 사용 중인 프로세스를 종료합니다.
 */
void kill_process_by_pid(pid_t pid) {
  if (kill(pid, SIGKILL) == 0) {
    printf("PID %d 프로세스를 성공적으로 종료했습니다.\n", pid);
  } else {
    printf("PID %d 프로세스 종료 실패: 권한 부족 또는 잘못된 PID입니다.\n",
           pid);
  }
}
