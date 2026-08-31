CXX = g++

# $(OBJECT_LIBXDP) $(OBJECT_LIBBPF)
include ./common_mk/common.mk

LIBBPF_LIBXDP_DIR = $(LIB_DIR)/install/include
COMMON_HDRS := ../common

K_INCLUDE_DIR := -I./ -I$(COMMON_HDRS) -I$(LIBBPF_LIBXDP_DIR)
K_CFLAGS := -Wall -Werror -fno-strict-aliasing -O3 -g
K_LDLIBS := $(OBJECT_LIBXDP) $(OBJECT_LIBBPF) -lpthread -ltcmalloc -lrt -lelf -lz

COMMON_DIR := ../common
RUNTIME_DIR := ./runtime
UTILS_DIR := ./utils

COMMON_TARGETS_SRC := $(wildcard $(COMMON_DIR)/*/*.cc)
COMMON_TARGETS_DEPS = $(COMMON_TARGETS_SRC:.cc=.d)
COMMON_TARGETS_OBJS += $(COMMON_TARGETS_SRC:.cc=.o)

RUNTIME_TARGETS_SRC := $(wildcard $(RUNTIME_DIR)/*.cc)
RUNTIME_TARGETS_DEPS := $(RUNTIME_TARGETS_SRC:.cc=.d)
RUNTIME_TARGETS_OBJS += $(RUNTIME_TARGETS_SRC:.cc=.o)

UTILS_TARGETS_SRC := $(wildcard $(UTILS_DIR)/*.cc)
UTILS_TARGETS_DEPS := $(UTILS_TARGETS_SRC:.cc=.d)
UTILS_TARGETS_OBJS += $(UTILS_TARGETS_SRC:.cc=.o)

KERNEL_SRC := $(wildcard *.cc)
KERNEL_DEPS := $(KERNEL_SRC:.cc=.d)
KERNEL_OBJS := $(KERNEL_SRC:.cc=.o)

all: bpf micro_kernel

micro_kernel: $(COMMON_TARGETS_OBJS) $(RUNTIME_TARGETS_OBJS) $(UTILS_TARGETS_OBJS) $(KERNEL_OBJS)
	$(CXX) $(K_CFLAGS) $(K_INCLUDE_DIR) -o $@ $^ $(K_LDLIBS)

$(COMMON_TARGETS_OBJS): $(OBJECT_LIBXDP)
$(RUNTIME_TARGETS_OBJS): $(OBJECT_LIBXDP)
$(KERNEL_OBJS): $(OBJECT_LIBXDP)

bpf: micro_kernel
	make -C eBPF

%.o: %.cc 
	$(CXX) $(K_CFLAGS) $(K_INCLUDE_DIR) -c -o $@ $<

.PHONY: clean
clean::
	make -C eBPF clean
	rm -f micro_kernel $(COMMON_TARGETS_OBJS) $(RUNTIME_TARGETS_OBJS) $(UTILS_TARGETS_OBJS) $(KERNEL_OBJS) $(COMMON_TARGETS_DEPS) $(RUNTIME_TARGETS_DEPS) $(UTILS_TARGETS_DEPS) $(KERNEL_DEPS)

-include $(COMMON_TARGETS_DEPS)
-include $(RUNTIME_TARGETS_DEPS)
-include $(UTILS_TARGETS_DEPS)
-include $(KERNEL_DEPS)

########## rules for generating .d files ##########
%.d: %.cc
	@set -e; \
	rm -f $@; \
	$(CXX) -MM $(K_INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;
