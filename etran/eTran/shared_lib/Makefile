# This file is used to generate shared library for eTran
CXX = g++
CC = gcc

SHARED_LIB = libetran.so

COMMON_DIR := ../common
APP_LIB := ../lib
LIBBPF_LIBXDP_DIR:= ../../lib/install/include

# $(OBJECT_LIBXDP) $(OBJECT_LIBBPF)
include ./common_mk/common.mk

CFLAGS := -Wall -Werror -fno-strict-aliasing -O3 -g -Wno-pointer-arith
INCLUDE_DIR := -I$(COMMON_DIR) -I$(APP_LIB)/include -I$(LIBBPF_LIBXDP_DIR)
LDLIBS := -lpthread -ltcmalloc -lrt -lelf -lz

COMMON_TARGETS_SRC := $(wildcard $(COMMON_DIR)/*/*.cc)
COMMON_TARGETS_DEPS = $(COMMON_TARGETS_SRC:.cc=.d)
COMMON_TARGETS_OBJS += $(COMMON_TARGETS_SRC:.cc=.o)

APP_LIB_TARGETS_SRC := $(wildcard $(APP_LIB)/*.cc)
APP_LIB_TARGETS_DEPS = $(APP_LIB_TARGETS_SRC:.cc=.d)
APP_LIB_TARGETS_OBJS += $(APP_LIB_TARGETS_SRC:.cc=.o)

all: $(SHARED_LIB)
$(SHARED_LIB): $(COMMON_TARGETS_OBJS) $(APP_LIB_TARGETS_OBJS)
	$(CXX) -shared -fPIC -o $@ $^ $(LDLIBS)

$(COMMON_TARGETS_OBJS): $(OBJECT_LIBXDP)
$(APP_LIB_TARGETS_OBJS): $(OBJECT_LIBXDP)

%.o: %.cc
	$(CXX) -fPIC $(CFLAGS) $(INCLUDE_DIR) -o $@ $< -c

%.o: %.c
	$(CC) -fPIC $(CFLAGS) $(INCLUDE_DIR) -o $@ $< -c

.phony: clean
clean:
	$(Q)rm -rf $(SHARED_LIB) $(COMMON_TARGETS_OBJS) $(COMMON_TARGETS_DEPS) $(APP_LIB_TARGETS_OBJS) $(APP_LIB_TARGETS_DEPS)

-include $(COMMON_TARGETS_DEPS)
-include $(APP_LIB_TARGETS_DEPS)

########## rules for generating .d files ##########
%.d: %.c
	@set -e; \
	rm -f $@; \
	$(CC) -MM $(INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;

%.d: %.cc
	@set -e; \
	rm -f $@; \
	$(CXX) -MM $(INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;