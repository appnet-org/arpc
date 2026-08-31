# Compile eBPF code with the makefile from bpf-examples
eBPF_DIR	:= ./

BPF_TARGETS	:= $(eBPF_DIR)/main_case1 $(eBPF_DIR)/main_case2 \
		   $(eBPF_DIR)/main_case3 $(eBPF_DIR)/main_case4
BPF_HEADERS	:= $(wildcard $(eBPF_DIR)/*.h) ../../../common/intf/intf_ebpf.h ../../../common/tran_def/tcp.h ../*.h

BPF_CFLAGS := -Wno-address-of-packed-member -I../../../intf -I../../../common/

LIB_DIR = ../../../../lib
include $(LIB_DIR)/common.mk