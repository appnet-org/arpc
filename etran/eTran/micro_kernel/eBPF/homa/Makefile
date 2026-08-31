# Compile eBPF code with the makefile from bpf-examples
eBPF_DIR	:= ./

BPF_TARGETS	:= $(eBPF_DIR)/main
BPF_HEADERS	:= $(wildcard $(eBPF_DIR)/*.h) ../../../common/intf/intf_ebpf.h ../../../common/tran_def/homa.h ../*.h

BPF_CFLAGS := -Wno-address-of-packed-member -I../../../intf -I../../../common/ -I../

LIB_DIR = ../../../../lib
include $(LIB_DIR)/common.mk