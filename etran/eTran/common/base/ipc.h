#pragma once

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/eventfd.h>
#include <unistd.h>
#include <errno.h>

int transfer_fd(int sockfd, int fd);
int receive_fd(int sockfd, int *fd);

int transfer_fds(int sockfd, int* fds, int fd_count);
int receive_fds(int sockfd, int* fds, int fd_count);

ssize_t write_all(int fd, const void *buffer, size_t count);
ssize_t read_all(int fd, void *buffer, size_t count);

static inline void kick_evfd(int evfd)
{
    uint64_t u = 1;
    ssize_t ret = write(evfd, &u, sizeof(u));
    (void)ret;
}

static inline uint64_t consume_evfd(int evfd)
{
    uint64_t u;
    ssize_t ret = read(evfd, &u, sizeof(u));
    if (ret != sizeof(u))
        return 0;
    return u;
}
