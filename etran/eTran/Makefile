.PHONY: all clean

all:
	$(MAKE) -C ../
	$(MAKE) -C shared_lib
	$(MAKE) -C micro_kernel
	$(MAKE) -C tcp_app
	$(MAKE) -C homa_app

clean:
	$(MAKE) -C shared_lib clean
	$(MAKE) -C micro_kernel clean
	$(MAKE) -C tcp_app clean
	$(MAKE) -C homa_app clean