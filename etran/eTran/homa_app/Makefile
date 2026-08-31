CXX = g++
CC = gcc

LIB_DIR:= ../../lib
# Include $(OBJECT_LIBXDP), $(OBJECT_LIBBPF)
include ./common_mk/common.mk

LIBBPF_LIBXDP_HDRS = $(LIB_DIR)/install/include
COMMON_HDRS := ../common
APPLIB_HDRS := ../lib/include

CP_NODE := cp_node

COMMON_DIR := ../common
COMMON_TARGETS_SRC := $(wildcard $(COMMON_DIR)/*/*.cc)
COMMON_TARGETS_DEPS = $(COMMON_TARGETS_SRC:.cc=.d)
COMMON_TARGETS_OBJS += $(COMMON_TARGETS_SRC:.cc=.o)

CP_NODE_DIR := ../lib
CP_NODE_RUNTIME_SRC := $(wildcard $(CP_NODE_DIR)/*.cc)
CP_NODE_RUNTIME_DEPS = $(CP_NODE_RUNTIME_SRC:.cc=.d)
CP_NODE_RUNTIME_OBJS += $(CP_NODE_RUNTIME_SRC:.cc=.o)

CP_NODE_INCLUDE_DIR := -I./ -I$(COMMON_HDRS) -I$(APPLIB_HDRS) -I$(LIBBPF_LIBXDP_HDRS)
CP_NODE_CFLAGS := -Wall -Werror -fno-strict-aliasing -O3 -g -Wno-pointer-arith
CP_NODE_LDLIBS := $(OBJECT_LIBXDP) $(OBJECT_LIBBPF) -lpthread -ltcmalloc -lrt -lelf -lz
CP_NODE_OBJS := dist.o test_utils.o time_trace.o homa_api.o cp_node.o

all: $(CP_NODE)

$(CP_NODE): %: $(COMMON_TARGETS_OBJS) $(CP_NODE_RUNTIME_OBJS) $(CP_NODE_OBJS) $(APPLIBHDR)
	$(CXX) $(CP_NODE_CFLAGS) $(CP_NODE_INCLUDE_DIR) -o $@ $^ $(CP_NODE_LDLIBS) 

%.o: %.c
	$(CC) $(CP_NODE_CFLAGS) $(CP_NODE_INCLUDE_DIR) -c -o $@ $<

%.o: %.cc
	$(CXX) $(CP_NODE_CFLAGS) $(CP_NODE_INCLUDE_DIR) -c -o $@ $<

.PHONY: clean
clean::
	$(Q)rm -f $(CP_NODE) $(COMMON_TARGETS_OBJS) $(CP_NODE_RUNTIME_OBJS) $(CP_NODE_OBJS) $(COMMON_TARGETS_DEPS) $(CP_NODE_RUNTIME_DEPS) 

-include $(COMMON_TARGETS_DEPS)
-include $(CP_NODE_RUNTIME_DEPS)

########## rules for .d files ##########
%.d: %.c
	@set -e; \
	rm -f $@; \
	$(CC) -MM $(CP_NODE_INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;

%.d: %.cc
	@set -e; \
	rm -f $@; \
	$(CXX) -MM $(CP_NODE_INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;
