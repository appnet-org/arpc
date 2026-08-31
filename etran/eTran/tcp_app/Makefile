CXX = g++
CC = gcc

A_CFLAGS := -Wall -Werror -fno-strict-aliasing -O3 -g -Wno-pointer-arith -Wno-address-of-packed-member
A_LDLIBS := -lpthread -ltcmalloc -lrt -lelf -lz

# List of applications
APPS := epoll_client epoll_server \
		lat_client lat_server \

OTHER_APPS := flexkvs_server flexkvs_bench
OTHER_APPS_OBJS := $(OTHER_APPS:=.o)

APP_CPP_SRCS := $(addsuffix .cc, $(APPS))
APP_CPP_OBJS := $(APP_CPP_SRCS:.cc=.o)
APP_CPP_DEPS := $(APP_CPP_SRCS:.cc=.d)

FLEXKVS_COMMON=flexkvs/utils.o flexkvs/hashtable.o flexkvs/ialloc.o flexkvs/jenkins_hash.o flexkvs/settings.o
BENCH_COMMON=flexkvs/rng.o flexkvs/commandline.o flexkvs/workload.o

all: $(APPS) $(OTHER_APPS)

$(APPS): %: %.o
	$(CXX) -o $@ $^ $(A_LDLIBS)

flexkvs_server: %: $(FLEXKVS_COMMON) %.o
	$(CXX) $(A_CFLAGS) -o $@ $^ $(A_LDLIBS) 

flexkvs_bench: %: $(BENCH_COMMON) $(FLEXKVS_COMMON) %.o
	$(CXX) $(A_CFLAGS) -o $@ $^ $(A_LDLIBS) 

%.o: %.cc
	$(CXX) -fPIC $(A_CFLAGS) $(A_INCLUDE_DIR) -c -o $@ $<

%.o: %.c
	$(CC) -fPIC $(A_CFLAGS) $(A_INCLUDE_DIR) -c -o $@ $<

.PHONY: clean
clean::
	$(Q)rm -f $(APPS) $(APP_CPP_OBJS) $(APP_CPP_DEPS) $(OTHER_APPS) $(OTHER_APPS_OBJS) $(FLEXKVS_COMMON) $(BENCH_COMMON)

-include $(APP_CPP_DEPS)

########## rules for generating .d files ##########
%.d: %.c
	@set -e; \
	rm -f $@; \
	$(CC) -MM $(A_INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;

%.d: %.cc
	@set -e; \
	rm -f $@; \
	$(CXX) -MM $(A_INCLUDE_DIR) $< > $@.tmp; \
	sed 's,\(.*\)\.o\:,$*\.o $*\.d\:,g' < $@.tmp > $@; \
	rm -f $@.tmp;
