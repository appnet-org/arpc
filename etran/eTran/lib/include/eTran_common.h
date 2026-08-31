#ifndef ETRAN_H
#define ETRAN_H

constexpr char MICRO_KERNEL_SOCK_PATH[] = "/tmp/micro_kernel_socket";
constexpr unsigned int RX_BATCH_SIZE = 32;
constexpr unsigned int TX_BATCH_SIZE = 32;
constexpr unsigned int CQ_BATCH_SIZE = 32;

int eTran_init(struct eTran_cfg *cfg);
void eTran_intr(int sig);
void eTran_exit(void);

struct app_ctx_per_thread *eTran_get_tctx(void);

// Statistics API
void eTran_dump_io_stats(struct app_ctx_per_thread *tctx);
void eTran_dump_all_io_stats(void);

#endif