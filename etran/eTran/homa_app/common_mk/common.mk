# get list of objects in util
include $(LIB_DIR)/util/util.mk

include $(LIB_DIR)/defines.mk

# Detect submodule libbpf source file changes
ifeq ($(SYSTEM_LIBBPF),n)
	LIBBPF_SOURCES := $(wildcard $(LIBBPF_DIR)/src/*.[ch])
endif

all: $(OBJECT_LIBBPF) $(OBJECT_LIBXDP)

$(OBJECT_LIBXDP): $(OBJECT_LIBBPF) $(LIBXDP_SOURCES)
	$(Q)$(MAKE) -C $(LIB_DIR) libxdp

$(OBJECT_LIBBPF): $(LIBBPF_SOURCES)
	$(Q)$(MAKE) -C $(LIB_DIR) libbpf


