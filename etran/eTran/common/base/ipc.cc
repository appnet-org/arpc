#include "ipc.h"

/**
 * @brief this function ensures that all data in the buffer is written to the file descriptor
 */
ssize_t write_all(int fd, const void *buffer, size_t count)
{
    const char *buf = reinterpret_cast<const char *>(buffer);
    ssize_t total_written = 0;

    while (count > 0) {
        ssize_t written = write(fd, buf, count);
        if (written == -1) {
            if (errno == EINTR) {
                continue;
            } else {
                return -1;
            }
        }

        count -= written;
        buf += written;
        total_written += written;
    }

    return total_written;
}

/**
 * @brief this function ensures that all data in the buffer is read from the file descriptor
 */
ssize_t read_all(int fd, void *buffer, size_t count) 
{
    char *buf = reinterpret_cast<char *>(buffer);
    ssize_t total_read = 0;

    while (count > 0) {
        ssize_t num_read = read(fd, buf, count);
        if (num_read == -1) {
            if (errno == EINTR) {
                continue;
            } else {
                return -1;
            }
        }
        if (num_read == 0) {
            break;  // EOF reached
        }

        count -= num_read;
        buf += num_read;
        total_read += num_read;
    }

    return total_read;
}

/**
 * @brief send fd to another process through sockfd
 *
 * @param sockfd
 * @param fd
 * @return int
 */
int transfer_fd(int sockfd, int fd)
{
    assert(sockfd >= 0);
    assert(fd >= 0);
    struct msghdr msg;
    struct cmsghdr *cmsg;
    struct iovec iov;
    char buf[CMSG_SPACE(sizeof(fd))];
    memset(&msg, 0, sizeof(msg));
    memset(buf, 0, sizeof(buf));
    const char *name = "fd";
    iov.iov_base = (void *)name;
    iov.iov_len = 4;
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;

    msg.msg_control = buf;
    msg.msg_controllen = sizeof(buf);

    cmsg = CMSG_FIRSTHDR(&msg);

    cmsg->cmsg_level = SOL_SOCKET;
    cmsg->cmsg_type = SCM_RIGHTS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(fd));

    *((int *)CMSG_DATA(cmsg)) = fd;

    msg.msg_controllen = CMSG_SPACE(sizeof(fd));

    if (sendmsg(sockfd, &msg, 0) < 0)
    {
        fprintf(stderr, "sendmsg failed\n");
        return -1;
    }
    return 0;
}

// multiple fds version
int transfer_fds(int sockfd, int* fds, int fd_count) {
    assert(sockfd >= 0);
    assert(fds != NULL);
    assert(fd_count > 0);

    struct msghdr msg;
    struct cmsghdr *cmsg;
    struct iovec iov;
    char buf[CMSG_SPACE(sizeof(int) * fd_count)];
    memset(&msg, 0, sizeof(msg));
    memset(buf, 0, sizeof(buf));
    const char *name = "fds";
    iov.iov_base = (void *)name;
    iov.iov_len = 4;
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;

    msg.msg_control = buf;
    msg.msg_controllen = sizeof(buf);

    cmsg = CMSG_FIRSTHDR(&msg);
    cmsg->cmsg_level = SOL_SOCKET;
    cmsg->cmsg_type = SCM_RIGHTS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(int) * fd_count);

    memcpy((int *)CMSG_DATA(cmsg), fds, sizeof(int) * fd_count);

    msg.msg_controllen = cmsg->cmsg_len;

    if (sendmsg(sockfd, &msg, 0) < 0) {
        fprintf(stderr, "sendmsg failed\n");
        return -1;
    }

    return 0;
}

/**
 * @brief receive fd from another process through sockfd
 *
 * @param sockfd
 * @param fd
 * @return int
 */
int receive_fd(int sockfd, int *fd)
{
    assert(sockfd >= 0);
    struct msghdr msg;
    struct iovec iov;
    char buf[CMSG_SPACE(sizeof(int))];
    struct cmsghdr *cmsg;

    iov.iov_base = buf;
    iov.iov_len = sizeof(buf);

    msg.msg_name = 0;
    msg.msg_namelen = 0;

    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;

    msg.msg_control = buf;
    msg.msg_controllen = sizeof(buf);
    if (recvmsg(sockfd, &msg, 0) < 0)
    {
        perror("recvmsg failed\n");
        return -1;
    }
    cmsg = CMSG_FIRSTHDR(&msg);
    if (cmsg == NULL || cmsg->cmsg_type != SCM_RIGHTS)
    {
        perror("recvmsg failed\n");
        return -1;
    }
    *fd = *((int *)CMSG_DATA(cmsg));
    return 0;
}

// multiple fds version
int receive_fds(int sockfd, int* fds, int fd_count) {
    assert(sockfd >= 0);
    assert(fds != NULL);
    assert(fd_count > 0);

    struct msghdr msg;
    struct iovec iov;
    char buf[CMSG_SPACE(sizeof(int) * fd_count)];
    struct cmsghdr *cmsg;

    iov.iov_base = buf;
    iov.iov_len = sizeof(buf);

    msg.msg_name = 0;
    msg.msg_namelen = 0;

    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;

    msg.msg_control = buf;
    msg.msg_controllen = sizeof(buf);
    if (recvmsg(sockfd, &msg, 0) < 0) {
        perror("recvmsg failed\n");
        return -1;
    }

    cmsg = CMSG_FIRSTHDR(&msg);
    if (cmsg == NULL || cmsg->cmsg_type != SCM_RIGHTS) {
        perror("recvmsg failed\n");
        return -1;
    }

    memcpy(fds, (int *)CMSG_DATA(cmsg), sizeof(int) * fd_count);

    return 0;
}