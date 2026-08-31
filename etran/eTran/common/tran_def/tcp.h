#pragma once

#include <stdint.h>

#include <linux/types.h>

/******************************************************************************/
/* Ethernet */

#define ETH_ALEN 6
#define ETH_TYPE_IP   0x0800

typedef uint16_t beui16_t;
typedef uint32_t beui32_t;

struct eth_addr {
  uint8_t addr[ETH_ALEN];
} __attribute__ ((packed));

struct eth_hdr {
  struct eth_addr dest;
  struct eth_addr src;
  beui16_t type;
} __attribute__ ((packed));

/******************************************************************************/
/* IPv4 */

#define IPH_V(hdr)  ((hdr)->_v_hl >> 4)
#define IPH_HL(hdr) ((hdr)->_v_hl & 0x0f)
#define IPH_TOS(hdr) ((hdr)->_tos)
#define IPH_ECN(hdr) ((hdr)->_tos & 0x3)

#define IPH_VHL_SET(hdr, v, hl) (hdr)->_v_hl = (((v) << 4) | (hl))
#define IPH_TOS_SET(hdr, tos) (hdr)->_tos = (tos)
#define IPH_ECN_SET(hdr, e) (hdr)->_tos = ((hdr)->_tos & 0xffc) | (e)

#define IP_HLEN 20

#define IP_PROTO_IP      0
#define IP_PROTO_ICMP    1
#define IP_PROTO_IGMP    2
#define IP_PROTO_IPENCAP 4
#define IP_PROTO_UDP     17
#define IP_PROTO_UDPLITE 136
#define IP_PROTO_TCP     6
#define IP_PROTO_DCCP	 33

/*
 * ECN (Explicit Congestion Notification) codepoints in RFC3168 mapped to the
 * lower 2 bits of the TOS field.
 */
#define	IPTOS_ECN_NOTECT	0x00	/* not-ECT */
#define	IPTOS_ECN_ECT1		0x01	/* ECN-capable transport (1) */
#define	IPTOS_ECN_ECT0		0x02	/* ECN-capable transport (0) */
#define	IPTOS_ECN_CE		0x03	/* congestion experienced */
#define	IPTOS_ECN_MASK		0x03	/* ECN field mask */

typedef beui32_t ip_addr_t;

struct ip_hdr {
  /* version / header length */
  uint8_t _v_hl;
  /* type of service */
  uint8_t _tos;
  /* total length */
  beui16_t len;
  /* identification */
  beui16_t id;
  /* fragment offset field */
  beui16_t offset;
  /* time to live */
  uint8_t ttl;
  /* protocol*/
  uint8_t proto;
  /* checksum */
  uint16_t chksum;
  /* source and destination IP addresses */
  ip_addr_t src;
  ip_addr_t dest;
} __attribute__ ((packed));

/******************************************************************************/
/* TCP */

#define TCP_MSS 1460

#define TCP_FIN 0x01U
#define TCP_SYN 0x02U
#define TCP_RST 0x04U
#define TCP_PSH 0x08U
#define TCP_ACK 0x10U
#define TCP_URG 0x20U
#define TCP_ECE 0x40U
#define TCP_CWR 0x80U
#define TCP_NS  0x100U

#define TCP_FLAGS 0x1ffU

/* Length of the TCP header, excluding options. */
#define TCP_HLEN 20

#define TCPH_HDRLEN(phdr) (ntohs((phdr)->_hdrlen_rsvd_flags) >> 12)
#define TCPH_FLAGS(phdr)  (ntohs((phdr)->_hdrlen_rsvd_flags) & TCP_FLAGS)

#define TCPH_HDRLEN_SET(phdr, len) (phdr)->_hdrlen_rsvd_flags = htons(((len) << 12) | TCPH_FLAGS(phdr))
#define TCPH_FLAGS_SET(phdr, flags) (phdr)->_hdrlen_rsvd_flags = (((phdr)->_hdrlen_rsvd_flags & PP_HTONS((uint16_t)(~(uint16_t)(TCP_FLAGS)))) | htons(flags))
#define TCPH_HDRLEN_FLAGS_SET(phdr, len, flags) (phdr)->_hdrlen_rsvd_flags = htons(((len) << 12) | (flags))

#define TCPH_SET_FLAG(phdr, flags ) (phdr)->_hdrlen_rsvd_flags = ((phdr)->_hdrlen_rsvd_flags | htons(flags))
#define TCPH_UNSET_FLAG(phdr, flags) (phdr)->_hdrlen_rsvd_flags = htons(ntohs((phdr)->_hdrlen_rsvd_flags) | (TCPH_FLAGS(phdr) & ~(flags)) )

#define TCP_TCPLEN(seg) ((seg)->len + ((TCPH_FLAGS((seg)->tcphdr) & (TCP_FIN | TCP_SYN)) != 0))

/** This returns a TCP header option for MSS in an u32_t */
#define TCP_BUILD_MSS_OPTION(mss) htonl(0x02040000 | ((mss) & 0xFFFF))

#define TCP_BUILD_SACK_OPTION	htonl(0x04020101)

struct tcp_hdr {
  beui16_t src;
  beui16_t dest;
  beui32_t seqno;
  beui32_t ackno;
  uint16_t _hdrlen_rsvd_flags;
  beui16_t wnd;
  uint16_t chksum;
  beui16_t urgp;
} __attribute__((packed));

#define TCP_OPT_END_OF_OPTIONS 0
#define TCP_OPT_NO_OP 1
#define TCP_OPT_MSS 2
#define TCP_OPT_TIMESTAMP 8
struct tcp_mss_opt {
  uint8_t kind;
  uint8_t length;
  beui16_t mss;
} __attribute__((packed));

struct tcp_timestamp_opt {
  uint8_t kind;
  uint8_t length;
  beui32_t ts_val;
  beui32_t ts_ecr;
} __attribute__((packed));

struct tcp_opts {
  struct tcp_mss_opt *mss;
  struct tcp_timestamp_opt *ts;
};

struct pkt_ip {
    struct eth_hdr eth;
    struct ip_hdr ip;
} __attribute__((packed));

struct pkt_tcp {
    struct eth_hdr eth;
    struct ip_hdr ip;
    struct tcp_hdr tcp;
} __attribute__((packed));

static inline unsigned short from32to16(unsigned int x)
{
	/* add up 16-bit and 16-bit for 16+c bit */
	x = (x & 0xffff) + (x >> 16);
	/* add up carry.. */
	x = (x & 0xffff) + (x >> 16);
	return x;
}

static inline unsigned int do_csum(const unsigned char *buff, int len)
{
	unsigned int result = 0;
	int odd;

	if (len <= 0)
		goto out;
	odd = 1 & (unsigned long)buff;
	if (odd) {
#ifdef __LITTLE_ENDIAN
		result += (*buff << 8);
#else
		result = *buff;
#endif
		len--;
		buff++;
	}
	if (len >= 2) {
		if (2 & (unsigned long)buff) {
			result += *(unsigned short *)buff;
			len -= 2;
			buff += 2;
		}
		if (len >= 4) {
			const unsigned char *end = buff +
						   ((unsigned int)len & ~3);
			unsigned int carry = 0;

			do {
				unsigned int w = *(unsigned int *)buff;

				buff += 4;
				result += carry;
				result += w;
				carry = (w > result);
			} while (buff < end);
			result += carry;
			result = (result & 0xffff) + (result >> 16);
		}
		if (len & 2) {
			result += *(unsigned short *)buff;
			buff += 2;
		}
	}
	if (len & 1)
#ifdef __LITTLE_ENDIAN
		result += *buff;
#else
		result += (*buff << 8);
#endif
	result = from32to16(result);
	if (odd)
		result = ((result >> 8) & 0xff) | ((result & 0xff) << 8);
out:
	return result;
}

static inline __sum16 ip_fast_csum(const void *iph, unsigned int ihl)
{
	return (__sum16)~do_csum((const unsigned char *)iph, ihl * 4);
}

static inline __sum16 csum_fold(__wsum csum)
{
	uint32_t sum = (uint32_t)csum;

	sum = (sum & 0xffff) + (sum >> 16);
	sum = (sum & 0xffff) + (sum >> 16);
	return (__sum16)~sum;
}

static inline uint32_t from64to32(uint64_t x)
{
	/* add up 32-bit and 32-bit for 32+c bit */
	x = (x & 0xffffffff) + (x >> 32);
	/* add up carry.. */
	x = (x & 0xffffffff) + (x >> 32);
	return (uint32_t)x;
}

static inline __wsum csum_tcpudp_nofold(__be32 saddr, __be32 daddr,
			  __u32 len, __u8 proto, __wsum sum)
{
	unsigned long long s = (uint32_t)sum;

	s += (uint32_t)saddr;
	s += (uint32_t)daddr;
#ifdef __BIG_ENDIAN__
	s += proto + len;
#else
	s += (proto + len) << 8;
#endif
	return (__wsum)from64to32(s);
}

static inline __sum16
csum_tcpudp_magic(__be32 saddr, __be32 daddr, __u32 len,
		  __u8 proto, __wsum sum)
{
	return csum_fold(csum_tcpudp_nofold(saddr, daddr, len, proto, sum));
}

static inline uint16_t udp_csum(uint32_t saddr, uint32_t daddr, uint32_t len,
			   uint8_t proto, uint16_t *udp_pkt)
{
	uint32_t csum = 0;
	uint32_t cnt = 0;

	/* udp hdr and data */
	for (; cnt < len; cnt += 2)
		csum += udp_pkt[cnt >> 1];

	return csum_tcpudp_magic(saddr, daddr, len, proto, csum);
}

static inline uint16_t tcp_csum(uint32_t saddr, uint32_t daddr, uint32_t len,
                uint8_t proto, uint8_t *tcp_pkt)
{
    uint32_t csum = 0;
    uint32_t cnt = 0;

    /* tcp hdr and data */
    for (; cnt < len; cnt += 2)
        csum += tcp_pkt[cnt] << 8 | tcp_pkt[cnt + 1];

    return csum_tcpudp_magic(saddr, daddr, len, proto, csum);
}
