// SPDX-License-Identifier: GPL-2.0
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include "xdpsock.h"
#include "utils.h"

/* This XDP program is only needed for multi-buffer and XDP_SHARED_UMEM modes.
 * If you do not use these modes, libbpf can supply an XDP program for you.
 */
char LICENSE[] SEC("license") = "GPL";
struct {
	__uint(type, BPF_MAP_TYPE_XSKMAP);
	__uint(max_entries, MAX_SOCKS);
	__uint(key_size, sizeof(int));
	__uint(value_size, sizeof(int));
} xsks_map SEC(".maps");

struct hkey_t {
	__u32 key1;
	__u32 key2;
	__u32 key3;
	__u32 key4;
}__attribute__((packed));

struct my_struct_t {
	struct bpf_rb_node grant_list_link;  // 32B
  	struct bpf_refcount ref;             // 4B
	__u8 data;
	__u32 prio;
	struct hkey_t hkey;
}__attribute__((packed));

int num_socks = 0;
static unsigned int rr;

static bool bpf_cb(struct bpf_rb_node *node_a,
                      const struct bpf_rb_node *node_b) {
  struct my_struct_t *a;
  struct my_struct_t *b;

  a = container_of(node_a, struct my_struct_t, grant_list_link);
  b = container_of(node_b, struct my_struct_t, grant_list_link);
  return a->data != b->data;
}

static bool new_srpt_less(struct bpf_rb_node *node_a,
                      const struct bpf_rb_node *node_b) {
  struct my_struct_t *a;
  struct my_struct_t *b;

  a = container_of(node_a, struct my_struct_t, grant_list_link);
  b = container_of(node_b, struct my_struct_t, grant_list_link);
  if (a->prio != b->prio)
  	return a->prio < b->prio;
  if (a->data != b->data)
  	return a->data < b->data;
  return 1;
}

static bool srpt_less(struct bpf_rb_node *node_a,
                      const struct bpf_rb_node *node_b) {
  struct my_struct_t *a;
  struct my_struct_t *b;

  a = container_of(node_a, struct my_struct_t, grant_list_link);
  b = container_of(node_b, struct my_struct_t, grant_list_link);
  return a->data < b->data;
}

#define bss_public(name) SEC(".bss." #name) __attribute__((aligned(8)))
bss_public(B) struct bpf_spin_lock grant_list_lock;
bss_public(B) struct bpf_rb_root groot
    __contains(my_struct_t, grant_list_link);

// extern struct bpf_rb_node *bpf_rbtree_search_impl(struct bpf_rb_root *root, struct bpf_rb_node *node,
// 				    bool (same)(struct bpf_rb_node *a, const struct bpf_rb_node *b),
// 					void *meta__ign, u64 off) __ksym;

// extern struct bpf_rb_node *bpf_rbtree_search_less_impl(struct bpf_rb_root *root, struct bpf_rb_node *node,
// 				    bool (same)(struct bpf_rb_node *a, const struct bpf_rb_node *b),
// 					void *meta__ign, u64 off) __ksym;

// struct my_struct_t {
// 	int name;
// 	__u8 data;
// 	struct bpf_rb_node grant_list_link;  // 32B
//   	struct bpf_refcount ref;             // 4B
// };

// /* Convenience macro to wrap over bpf_rbtree_add_impl */
// #define bpf_rbtree_search(head, node, same) bpf_rbtree_search_impl(head, node, same, NULL, 0)
// #define bpf_rbtree_search_less(head, node, same) bpf_rbtree_search_less_impl(head, node, same, NULL, 0)
// #define bss_public(name) SEC(".bss." #name) __attribute__((aligned(8)))
// bss_public(B) struct bpf_spin_lock grant_list_lock;
// bss_public(B) struct bpf_rb_root groot
//     __contains(my_struct_t, grant_list_link);

// static bool same(struct bpf_rb_node *node_a,
//                       const struct bpf_rb_node *node_b) {
//   struct my_struct_t *a;
//   struct my_struct_t *b;

//   a = container_of(node_a, struct my_struct_t, grant_list_link);
//   b = container_of(node_b, struct my_struct_t, grant_list_link);
//   return a->name == b->name;
// }

// static bool less_name(struct bpf_rb_node *node_a,
//                       const struct bpf_rb_node *node_b) {
//   struct my_struct_t *a;
//   struct my_struct_t *b;

//   a = container_of(node_a, struct my_struct_t, grant_list_link);
//   b = container_of(node_b, struct my_struct_t, grant_list_link);
//   return a->name < b->name;
// }


// static bool less(struct bpf_rb_node *node_a,
//                       const struct bpf_rb_node *node_b) {
//   struct my_struct_t *a;
//   struct my_struct_t *b;

//   a = container_of(node_a, struct my_struct_t, grant_list_link);
//   b = container_of(node_b, struct my_struct_t, grant_list_link);
//   return a->data < b->data;
// }

// struct htbl_struct_t {
// 	int data1;
// 	int data2;
// };

// struct {
// 	__uint(type, BPF_MAP_TYPE_HASH);
// 	__uint(max_entries, 10);
// 	__uint(key_size, sizeof(int));
// 	__uint(value_size, sizeof(struct htbl_struct_t));
// } htbl SEC(".maps");

static __always_inline int test_rbtree(void)
{
	int ret = 0;
	struct bpf_rb_node *rb_node = NULL;
	struct my_struct_t *n = NULL;
	struct my_struct_t *n_ref_1 = NULL;
	struct my_struct_t *n_ref_2 = NULL;
	n = bpf_obj_new(typeof(*n));
	if (!n) {
		return XDP_DROP;
	}
	n_ref_1 = bpf_refcount_acquire(n);
	if (!n_ref_1) {
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	
	// add n_ref_1 to the grant list
	bpf_spin_lock(&grant_list_lock);
	ret = bpf_rbtree_add(&groot, &n_ref_1->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);
	bpf_printk("add n_ref_1 to the grant list ret = %d", ret);

	n_ref_2 = bpf_refcount_acquire(n);
	if (!n_ref_2) {
		bpf_obj_drop(n);
		return XDP_DROP;
	}

	// try to add n_ref_2 to the grant list when n_ref_1 still exists
	bpf_spin_lock(&grant_list_lock);
	ret = bpf_rbtree_add(&groot, &n_ref_2->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);
	bpf_printk("try to add n_ref_2 to the grant list when n_ref_1 still exists ret = %d", ret);

	bpf_spin_lock(&grant_list_lock);
	rb_node = bpf_rbtree_first(&groot);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	n_ref_1 = container_of(rb_node, struct my_struct_t, grant_list_link);
	rb_node = bpf_rbtree_remove(&groot, &n_ref_1->grant_list_link);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	n_ref_1 = container_of(rb_node, struct my_struct_t, grant_list_link);
	bpf_spin_unlock(&grant_list_lock);
	bpf_obj_drop(n_ref_1);

	// n_ref_2 is invalid no matter bpf_rbtree_add success or not
	n_ref_2 = bpf_refcount_acquire(n);
	if (!n_ref_2) {
		bpf_obj_drop(n);
		return XDP_DROP;
	}

	// try to add n_ref_2 to the grant list when n_ref_1 doesn't exists
	bpf_spin_lock(&grant_list_lock);
	ret = bpf_rbtree_add(&groot, &n_ref_2->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);
	bpf_printk("try to add n_ref_2 to the grant list when n_ref_1 doesn't exists ret = %d", ret);


	// bpf_obj_drop(n_ref_1);
	bpf_obj_drop(n);
	return XDP_DROP;
}

static __always_inline int test_rbtree_loop(void)
{
	int ret = 0;
	struct bpf_rb_node *rb_node = NULL;
	struct my_struct_t *n = NULL;

	#define CHECK_LIMIT 16
	#define CHECK_OK 10

	n = bpf_obj_new(typeof(*n));
	if (!n) {
		return XDP_DROP;
	}
	bpf_obj_drop(n);

	bpf_spin_lock(&grant_list_lock);

	rb_node = bpf_rbtree_first(&groot);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return XDP_DROP;
	}

	n = container_of(rb_node, struct my_struct_t, grant_list_link);

	int check_ok = 0;
	int check_limit = 0;
	while((rb_node = bpf_rbtree_next(&groot, &n->grant_list_link))) {
		n = container_of(rb_node, struct my_struct_t, grant_list_link);

		check_limit++;
		if (check_limit >= CHECK_LIMIT) break;
		
		if (n->data > 0)
			check_ok++;
		
		if (check_ok >= CHECK_OK) break;
	}

	bpf_spin_unlock(&grant_list_lock);

	return XDP_DROP;
}

bss_public(A) struct bpf_spin_lock grant_lock;

static __always_inline int test_rbtree_search(void)
{
	int ret = 0;
	struct bpf_rb_node *rb_node = NULL;

	struct my_struct_t c_n1;
	struct my_struct_t c_n2;
	struct my_struct_t c_n3;
	struct my_struct_t c_n4;
	struct my_struct_t c_n5;
	struct my_struct_t c_n6;
	struct my_struct_t c_n7;
	struct my_struct_t c_n8;
	struct my_struct_t c_n9;
	struct my_struct_t c_n10;

	struct my_struct_t *n = NULL;
	struct my_struct_t *n1 = NULL;
	struct my_struct_t *n2 = NULL;
	struct my_struct_t *n3 = NULL;
	struct my_struct_t *n4 = NULL;
	struct my_struct_t *n5 = NULL;
	struct my_struct_t *n6 = NULL;
	struct my_struct_t *n7 = NULL;
	struct my_struct_t *n8 = NULL;
	struct my_struct_t *n9 = NULL;
	struct my_struct_t *n10 = NULL;

	#define CHECK_OK 10

	n1 = bpf_obj_new(typeof(*n1));
	if (!n1) {
		return XDP_DROP;
	}

	n2 = bpf_obj_new(typeof(*n2));
	if (!n2) {
		bpf_obj_drop(n1);
		return XDP_DROP;
	}
	n3 = bpf_obj_new(typeof(*n3));
	if (!n3) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		return XDP_DROP;
	}
	n4 = bpf_obj_new(typeof(*n4));
	if (!n4) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		return XDP_DROP;
	}
	n5 = bpf_obj_new(typeof(*n5));
	if (!n5) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		bpf_obj_drop(n4);
		return XDP_DROP;
	}
	n6 = bpf_obj_new(typeof(*n6));
	if (!n6) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		bpf_obj_drop(n4);
		bpf_obj_drop(n5);
		return XDP_DROP;
	}
	n7 = bpf_obj_new(typeof(*n7));
	if (!n7) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		bpf_obj_drop(n4);
		bpf_obj_drop(n5);
		bpf_obj_drop(n6);
		return XDP_DROP;
	}
	n8 = bpf_obj_new(typeof(*n8));
	if (!n8) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		bpf_obj_drop(n4);
		bpf_obj_drop(n5);
		bpf_obj_drop(n6);
		bpf_obj_drop(n7);
		return XDP_DROP;
	}
	n9 = bpf_obj_new(typeof(*n9));
	if (!n9) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		bpf_obj_drop(n4);
		bpf_obj_drop(n5);
		bpf_obj_drop(n6);
		bpf_obj_drop(n7);
		bpf_obj_drop(n8);
		return XDP_DROP;
	}
	n10 = bpf_obj_new(typeof(*n10));
	if (!n10) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		bpf_obj_drop(n4);
		bpf_obj_drop(n5);
		bpf_obj_drop(n6);
		bpf_obj_drop(n7);
		bpf_obj_drop(n8);
		bpf_obj_drop(n9);
		return XDP_DROP;
	}

	// bpf_obj_drop(n1);
	// bpf_obj_drop(n2);
	// bpf_obj_drop(n3);
	// bpf_obj_drop(n4);
	// bpf_obj_drop(n5);
	// bpf_obj_drop(n6);
	// bpf_obj_drop(n7);
	// bpf_obj_drop(n8);
	// bpf_obj_drop(n9);
	// bpf_obj_drop(n10);

	bpf_spin_lock(&grant_list_lock);
	int i = 0;
	for (i = 0; i < CHECK_OK; i++) {
		if (i == 0)
			rb_node = bpf_rbtree_search(&groot, &n1->grant_list_link, bpf_cb);
		else if (i == 1)
			rb_node = bpf_rbtree_search(&groot, &n2->grant_list_link, bpf_cb);
		else if (i == 2)
			rb_node = bpf_rbtree_search(&groot, &n3->grant_list_link, bpf_cb);
		else if (i == 3)
			rb_node = bpf_rbtree_search(&groot, &n4->grant_list_link, bpf_cb);
		else if (i == 4)
			rb_node = bpf_rbtree_search(&groot, &n5->grant_list_link, bpf_cb);
		else if (i == 5)
			rb_node = bpf_rbtree_search(&groot, &n6->grant_list_link, bpf_cb);
		else if (i == 6)
			rb_node = bpf_rbtree_search(&groot, &n7->grant_list_link, bpf_cb);
		else if (i == 7)
			rb_node = bpf_rbtree_search(&groot, &n8->grant_list_link, bpf_cb);
		else if (i == 8)
			rb_node = bpf_rbtree_search(&groot, &n9->grant_list_link, bpf_cb);
		else
			rb_node = bpf_rbtree_search(&groot, &n10->grant_list_link, bpf_cb);
		if (!rb_node) continue;
		n = container_of(rb_node, struct my_struct_t, grant_list_link);
		
		if (i == 0)
			c_n1 = *n;
		else if (i == 1)
			c_n2 = *n;
		else if (i == 2)
			c_n3 = *n;
		else if (i == 3)
			c_n4 = *n;
		else if (i == 4)
			c_n5 = *n;
		else if (i == 5)
			c_n6 = *n;
		else if (i == 6)
			c_n7 = *n;
		else if (i == 7)
			c_n8 = *n;
		else if (i == 8)
			c_n9 = *n;
		else
			c_n10 = *n;
	}

	bpf_spin_unlock(&grant_list_lock);

	return XDP_DROP;
}

static __always_inline int test_rbtree_next(void)
{
	int ret = 0;
	struct bpf_rb_node *rb_node = NULL;
	struct my_struct_t *n = NULL;
	struct my_struct_t *n1 = NULL;
	struct my_struct_t *n2 = NULL;

	int read_n = 0;
	int read_n1 = 0;
	int read_n2 = 0;

	n = bpf_obj_new(typeof(*n));
	if (!n) {
		return XDP_DROP;
	}
	n->data = 0;

	n1 = bpf_obj_new(typeof(*n1));
	if (!n1) {
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	n1->data = 1;

	n2 = bpf_obj_new(typeof(*n2));
	if (!n2) {
		bpf_obj_drop(n);
		bpf_obj_drop(n1);
		return XDP_DROP;
	}
	n2->data = 2;

	bpf_spin_lock(&grant_list_lock);
	bpf_rbtree_add(&groot, &n->grant_list_link, srpt_less);
	bpf_rbtree_add(&groot, &n1->grant_list_link, srpt_less);
	bpf_rbtree_add(&groot, &n2->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);

	bpf_spin_lock(&grant_list_lock);

	rb_node = bpf_rbtree_first(&groot);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return XDP_DROP;
	}

	n = container_of(rb_node, struct my_struct_t, grant_list_link);

	read_n = n->data;

	rb_node = bpf_rbtree_next(&groot, &n->grant_list_link);

	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return XDP_DROP;
	}

	n1 = container_of(rb_node, struct my_struct_t, grant_list_link);

	read_n1 = n1->data;

	rb_node = bpf_rbtree_next(&groot, &n1->grant_list_link);

	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return XDP_DROP;
	}

	n2 = container_of(rb_node, struct my_struct_t, grant_list_link);

	rb_node = bpf_rbtree_remove(&groot, &n2->grant_list_link);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return XDP_DROP;
	}

	bpf_spin_unlock(&grant_list_lock);

	bpf_printk("read_n = %d, read_n1 = %d", read_n, read_n1);

	n2 = container_of(rb_node, struct my_struct_t, grant_list_link);
	bpf_printk("read_n2 = %d", n2->data);
	bpf_obj_drop(n2);

	return XDP_DROP;
}

static __always_inline int test_rbtree_2(void)
{
	int ret = 0;
	struct bpf_rb_node *rb_node = NULL;
	struct my_struct_t *n = NULL;
	struct my_struct_t *n_ref_1 = NULL;
	struct my_struct_t *n_ref_2 = NULL;
	n = bpf_obj_new(typeof(*n));
	if (!n) {
		return XDP_DROP;
	}
	n_ref_1 = bpf_refcount_acquire(n);
	if (!n_ref_1) {
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	n_ref_2 = bpf_refcount_acquire(n);
	if (!n_ref_2) {
		bpf_obj_drop(n);
		bpf_obj_drop(n_ref_1);
		return XDP_DROP;
	}
	
	// add n to the grant list
	bpf_spin_lock(&grant_list_lock);
	ret = bpf_rbtree_add(&groot, &n->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);
	bpf_printk("add n to the grant list ret = %d", ret);

	bpf_spin_lock(&grant_list_lock);
	ret = bpf_rbtree_add(&groot, &n_ref_1->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);
	bpf_printk("try to add n_ref_1 to the grant list when n exists in it, ret = %d", ret);

	bpf_spin_lock(&grant_list_lock);
	rb_node = bpf_rbtree_first(&groot);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		bpf_obj_drop(n_ref_2);
		return XDP_DROP;
	}
	n = container_of(rb_node, struct my_struct_t, grant_list_link);
	rb_node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		bpf_obj_drop(n_ref_2);
		return XDP_DROP;
	}
	bpf_spin_unlock(&grant_list_lock);

	n = container_of(rb_node, struct my_struct_t, grant_list_link);
	
	bpf_spin_lock(&grant_list_lock);
	ret = bpf_rbtree_add(&groot, &n_ref_2->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);
	bpf_printk("try to add n_ref_2 to the grant list when we use remove on n, ret = %d", ret);

	bpf_obj_drop(n);

	return XDP_DROP;
}

int static __always_inline test_rbtree_remove(void)
{
	struct bpf_rb_node *rb_node = NULL;
	struct my_struct_t *n = NULL;
	struct my_struct_t *ref_n = NULL;
	int ret = 0;

	n = bpf_obj_new(typeof(*n));
	if (!n) {
		return XDP_DROP;
	}

	ref_n = bpf_refcount_acquire(n);
	if (!ref_n) {
		bpf_obj_drop(n);
		return XDP_DROP;
	}

	bpf_spin_lock(&grant_list_lock);
	bpf_rbtree_add(&groot, &ref_n->grant_list_link, srpt_less);
	bpf_spin_unlock(&grant_list_lock);

	bpf_spin_lock(&grant_list_lock);
	rb_node = bpf_rbtree_first(&groot);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	ref_n = container_of(rb_node, struct my_struct_t, grant_list_link);
	rb_node = bpf_rbtree_remove(&groot, &ref_n->grant_list_link);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		bpf_obj_drop(n);
		return XDP_DROP;
	}
	ref_n = container_of(rb_node, struct my_struct_t, grant_list_link);

	// try to add n to rbtree
	ret = bpf_rbtree_add(&groot, &n->grant_list_link, srpt_less);

	bpf_spin_unlock(&grant_list_lock);

	bpf_obj_drop(ref_n);
	bpf_printk("Remove one's ref node, but not drop, try to insert the node, ret = %d", ret);
}

static __always_inline void test_lower_bound(void)
{
	struct my_struct_t *n = NULL;
	struct my_struct_t *n1 = NULL;
	struct my_struct_t *n2 = NULL;
	struct my_struct_t *n3 = NULL;
	struct my_struct_t *n4 = NULL;
	struct bpf_rb_node *rb_node = NULL;

	n1 = bpf_obj_new(typeof(*n1));
	if (!n1) {
		return;
	}

	n1->data = 2;
	n1->prio = 1000;

	n2 = bpf_obj_new(typeof(*n2));
	if (!n2) {
		bpf_obj_drop(n1);
		return;
	}

	n2->data = 4;
	n2->prio = 60000;

	n3 = bpf_obj_new(typeof(*n3));
	if (!n3) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		return;
	}

	n3->data = 6;
	n3->prio = 800000;

	n4 = bpf_obj_new(typeof(*n4));
	if (!n4) {
		bpf_obj_drop(n1);
		bpf_obj_drop(n2);
		bpf_obj_drop(n3);
		return;
	}

	__u8 search_data = 4;
	__u32 search_prio = 60000;
	n4->data = search_data;
	n4->prio = search_prio;

	bpf_spin_lock(&grant_list_lock);
	bpf_rbtree_add(&groot, &n1->grant_list_link, new_srpt_less);
	bpf_rbtree_add(&groot, &n2->grant_list_link, new_srpt_less);
	bpf_rbtree_add(&groot, &n3->grant_list_link, new_srpt_less);
	bpf_spin_unlock(&grant_list_lock);

	bool found = false;
	bpf_spin_lock(&grant_list_lock);
	rb_node = bpf_rbtree_lower_bound(&groot, &n4->grant_list_link, new_srpt_less);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return;
	}
	n = container_of(rb_node, struct my_struct_t, grant_list_link);
	rb_node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return;
	}
	n = container_of(rb_node, struct my_struct_t, grant_list_link);
	bpf_spin_unlock(&grant_list_lock);

	if (n->data == search_data && n->prio == search_prio) {
		found = true;
	}
	bpf_printk("n->data = %d, n->prio = %d", n->data, n->prio);

	bpf_obj_drop(n);

	bpf_printk("found = %d", found);

	bpf_spin_lock(&grant_list_lock);

	rb_node = bpf_rbtree_first(&groot);

	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return;
	}

	n = container_of(rb_node, struct my_struct_t, grant_list_link);
	rb_node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	if (!rb_node) {
		bpf_spin_unlock(&grant_list_lock);
		return;
	}
	n = container_of(rb_node, struct my_struct_t, grant_list_link);
	bpf_spin_unlock(&grant_list_lock);

	bpf_printk("n->data = %d, n->prio = %d", n->data, n->prio);
	bpf_obj_drop(n);

}

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1024);
	__uint(key_size, sizeof(int));
	__uint(value_size, sizeof(int));
} simple_map SEC(".maps");

static __always_inline void test_maximum_lookup(void)
{
	int key = 0;
	int *value = NULL;

	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
	value = bpf_map_lookup_elem(&simple_map, &key);
	if (!value) {
		bpf_printk("lookup failed");
		return;
	}
}

static __always_inline void test_struct(void)
{
	struct my_struct_t *n = NULL;
	n = bpf_obj_new(typeof(*n));
	if (!n) {
		return;
	}
	__builtin_memset(&n->hkey, 0, sizeof(struct hkey_t));
	n->hkey.key1 = 1;
	bpf_obj_drop(n);
}

// struct small_s {
//   __u64 rpcid;                         // 8B
//   __u32 remote_ip;                     // 4B
//   __u16 local_port;                    // 2B
//   __u16 remote_port;                   // 2B
// };

// struct large_s {
//   // === cache line ===
//   struct bpf_rb_node rbtree_link;      // 32B
//   struct small_s hkey;                  // 16B
//   struct bpf_refcount ref;             // 4B
//   __u32 bytes_remaining;               // 4B
//   __u32 incoming;                      // 4B
//   union {
//     __u16 peer_id;                       // 2B
//     // used by throttle list
//     __u16 slot_idx;
//   };

//   // used by throttle list
//   __u8 backyard;                      // 1B
                               
//   /**
//    * @brief tree_id = 0: rpc rbtree, tree_id = 1: peer rbtree
//    */
//   __u8 tree_id;                        // 1B
//   // === cache line ===
  
//   // we embed ***in_peer_tree*** to the lowest bit of birth
//   __u64 birth;                        // 8B

//   __u32 message_length;               // 4B

// }; //FIXME: compiler complains about this but ebpf verifier require
// // };

struct my_t {
	int a;
	int b;
};

void func(struct my_t *n_ptr) {
	n_ptr->a = 1;
	n_ptr->b = 2;
	bpf_printk("n_ptr->a = %d, n_ptr->b = %d", n_ptr->a, n_ptr->b);
}

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 65536);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} test_map SEC(".maps");

SEC(".bss.current_bucket_idx")
__u64 current_bucket_idx[20][8192];

SEC("xdp_sock") int xdp_sock_prog(struct xdp_md *ctx)
{	
    // for (int i = 0; i < 8192; i++) {
    //     __u32 key = i;
    //     __u32 *value = bpf_map_lookup_elem(&test_map, &key);
    //     if (!value) {
    //         bpf_printk("lookup failed");
    //         return XDP_DROP;
    //     }
    //     *value += 2;
    // }
    // for (int i = 0; i < 8192; i++) {
    //     for (int j = 0; j < 16;j++)
    //         current_bucket_idx[10][i]++;
    // }


	// int a = 0;
	// struct my_t n = {0};
	// struct my_t *n_ptr = &n;
	// n_ptr = NULL;
	// // case#1: verifier reject
	// func(n_ptr);
	// // case#2: verifier accept
	// func(&a);

	// struct large_s *ls;
	// ls = bpf_obj_new(typeof(*ls));
	// if (!ls) {
	// 	return XDP_DROP;
	// }
	// bpf_printk("sizeof(ls) = %d", sizeof(*ls));
	// ls->hkey.rpcid = 1;
	// ls->hkey.local_port = 1;
	// ls->hkey.remote_port = 1;
	// ls->hkey.remote_ip = 1;
	// bpf_obj_drop(ls);
	// test_struct();
	// test_maximum_lookup();
	// test_lower_bound();

	// #define SIZE 255
	// char test_stack_limit[SIZE];
	// char test_stack_limit_2[SIZE];

	// for (int i = 0; i < SIZE; i++)
	// 	test_stack_limit[i] = 256 - (i%256);

	// for (int i = 0; i < SIZE; i++)
	// 	test_stack_limit_2[i] = (test_stack_limit[i] + i)%256;

	// for (int i = 0; i < SIZE; i++)
	// 	test_stack_limit[i] = (test_stack_limit_2[i] -i)%256;

	// test_rbtree_remove();

	// bpf_spin_lock(&grant_lock);
	// bpf_spin_unlock(&grant_lock);
	// test_rbtree_search();
	// test_rbtree_loop();
	// test_rbtree_next();
	// test_rbtree_2();
	// int key = 0;
	// struct htbl_struct_t htbl_data;
	// htbl_data.data1 = 100;
	// htbl_data.data2 = 200;
	// bpf_map_update_elem(&htbl, &key, &htbl_data, BPF_ANY);

	// struct htbl_struct_t *htbl_data_ptr = bpf_map_lookup_elem(&htbl, &key);
	// if (!htbl_data_ptr) return XDP_DROP;

	// htbl_data_ptr->data1 = 300;

	// htbl_data_ptr = bpf_map_lookup_elem(&htbl, &key);
	// if (!htbl_data_ptr) return XDP_DROP;

	// bpf_printk("htbl_data_ptr->data1: %d, expect 300", htbl_data_ptr->data1);

	// bpf_map_delete_elem(&htbl, &key);

	// bpf_printk("htbl_data_ptr->data1: %d, expect 300", htbl_data_ptr->data1);

	// struct htbl_struct_t *htbl_data_ptr2 = bpf_map_lookup_elem(&htbl, &key);
	// if (!htbl_data_ptr2) {
	// 	bpf_printk("lookup failed");
	// 	return XDP_DROP;
	// }
	


	// int data = 0;
	// struct bpf_rb_node *rb_node = NULL;
	// struct my_struct_t *n = NULL;
	// struct my_struct_t *n1 = bpf_obj_new(typeof(*n1));
	// if (!n1) {
	// 	return XDP_DROP;
	// }
	// n1->data = 200;
	// struct my_struct_t *n2 = bpf_obj_new(typeof(*n2));
	// if (!n2) {
	// 	bpf_obj_drop(n1);
	// 	return XDP_DROP;
	// }
	// n2->data = 100;
	// struct my_struct_t *ref_n1 = bpf_refcount_acquire(n1);
	// if (!ref_n1) {
	// 	bpf_obj_drop(n1);
	// 	bpf_obj_drop(n2);
	// 	return XDP_DROP;
	// }
	// bpf_spin_lock(&grant_list_lock);
	// bpf_rbtree_add(&groot, &n1->grant_list_link, srpt_less);
	// bpf_rbtree_add(&groot, &n2->grant_list_link, srpt_less);
	// bpf_spin_unlock(&grant_list_lock);
	// ref_n1->data = 10;
	// bpf_obj_drop(ref_n1);

	// bpf_spin_lock(&grant_list_lock);
	// rb_node = bpf_rbtree_first(&groot);
	// if (!rb_node) {
	// 	bpf_spin_unlock(&grant_list_lock);
	// 	return XDP_DROP;
	// }
	// n = container_of(rb_node, struct my_struct_t, grant_list_link);
	// rb_node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	// if (!rb_node) {
	// 	bpf_spin_unlock(&grant_list_lock);
	// 	return XDP_DROP;
	// }
	// n = container_of(rb_node, struct my_struct_t, grant_list_link);
	// data = n->data;
	// bpf_spin_unlock(&grant_list_lock);
	// bpf_printk("data = %d, expect 100", data);

	// bpf_spin_lock(&grant_list_lock);
	// bpf_rbtree_add(&groot, &n->grant_list_link, srpt_less);
	// bpf_spin_unlock(&grant_list_lock);
	
	// bpf_spin_lock(&grant_list_lock);
	// rb_node = bpf_rbtree_first(&groot);
	// if (!rb_node) {
	// 	bpf_spin_unlock(&grant_list_lock);
	// 	return XDP_DROP;
	// }
	// n = container_of(rb_node, struct my_struct_t, grant_list_link);
	// rb_node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	// if (!rb_node) {
	// 	bpf_spin_unlock(&grant_list_lock);
	// 	return XDP_DROP;
	// }
	// n = container_of(rb_node, struct my_struct_t, grant_list_link);
	// data = n->data;
	// bpf_spin_unlock(&grant_list_lock);
	// bpf_printk("data = %d, expect 10", data);
	// bpf_obj_drop(n);

	// struct my_struct_t n;
	// struct my_struct_t *ptr = &n;
	// struct my_struct_t *ptr_2 = NULL;
	// struct my_struct_t *ptr_3 = NULL;
	// ptr->data = 100;
	// int key = 0;
	// bpf_map_update_elem(&my_data_map,&key, ptr, BPF_NOEXIST);
	// ptr->data = 200;

	// ptr_2 = bpf_map_lookup_elem(&my_data_map, &key);
	// if (ptr_2) {
	// 	bpf_printk("ptr_2->data: %d,\n", ptr_2->data);
	// 	ptr_2->data = 300;
	// 	ptr_3 = bpf_map_lookup_elem(&my_data_map, &key);
	// 	if (ptr_3) {
	// 		bpf_printk("ptr_3->data: %d,\n", ptr_3->data);
	// 	}
	// }

	// struct my_struct_t *n = NULL;
	// struct my_struct_t *nn = NULL;
	// struct my_struct_t *n1 = NULL;
	// struct bpf_rb_node *node = NULL;

	// n = bpf_obj_new(typeof(*n));
	// if (!n) return XDP_DROP;
	// n->name = 100;
	// n->data = 10;

	// nn = bpf_obj_new(typeof(*nn));
	// if (!nn) {
	// 	bpf_obj_drop(n);
	// 	return XDP_DROP;
	// }
	// nn->name = 50;
	// nn->data = 20;

	// n1 = bpf_obj_new(typeof(*n1));
	// if (!n1) {
	// 	bpf_obj_drop(n);
	// 	bpf_obj_drop(nn);
	// 	return XDP_DROP;
	// }
	// n1->name = __INT32_MAX__;
	// n1->data = 30;

	// bpf_spin_lock(&grant_list_lock);
	// bpf_rbtree_add(&groot, &n->grant_list_link, less);
	// bpf_rbtree_add(&groot, &nn->grant_list_link, less);
	// bpf_spin_unlock(&grant_list_lock);

	// bpf_spin_lock(&grant_list_lock);
	// node = bpf_rbtree_search_less(&groot, &n1->grant_list_link, less_name);

	// if (!node) {
	// 	bpf_spin_unlock(&grant_list_lock);
	// 	bpf_printk("Not found");
	// }
	// else {
	// 	n = container_of(node, struct my_struct_t, grant_list_link);
	// 	node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	// 	if (!node) {
	// 		bpf_spin_unlock(&grant_list_lock);
	// 	} else {
	// 		n = container_of(node, struct my_struct_t, grant_list_link);
	// 		bpf_spin_unlock(&grant_list_lock);
	// 		bpf_printk("Found: %d", n->name);
	// 		bpf_obj_drop(n);
	// 	}
	// }

	// bpf_spin_lock(&grant_list_lock);
	// node = bpf_rbtree_search(&groot, &n1->grant_list_link, same);

	// if (!node) {
	// 	bpf_spin_unlock(&grant_list_lock);
	// 	bpf_printk("Not found");
	// }
	// else {
	// 	n = container_of(node, struct my_struct_t, grant_list_link);
	// 	node = bpf_rbtree_remove(&groot, &n->grant_list_link);
	// 	if (!node) {
	// 		bpf_spin_unlock(&grant_list_lock);
	// 	} else {
	// 		n = container_of(node, struct my_struct_t, grant_list_link);
	// 		bpf_spin_unlock(&grant_list_lock);
	// 		bpf_printk("Found: %d", n->name);
	// 		bpf_obj_drop(n);
	// 	}
	// }

	rr = (rr + 1) & (num_socks - 1);
	return bpf_redirect_map(&xsks_map, rr, XDP_DROP);
}
