#pragma once

#include <linux/types.h>

#define IPPROTO_HOMA 0xFD

/* Maximum number of bytes in a Homa message. */
#define HOMA_MAX_MESSAGE_LENGTH 1000000

enum homa_packet_type {
    DATA = 0x10,
    GRANT = 0x11,
    RESEND = 0x12,
    UNKNOWN = 0x13,
    BUSY = 0x14,
    CUTOFFS = 0x15,
    FREEZE = 0x16,
    NEED_ACK = 0x17,
    ACK = 0x18,
    BOGUS = 0x19,
};

struct common_header {
    /**
     * @sport: Port on source machine from which packet was sent.
     * Must be in the same position as in a TCP header.
     */
    __be16 sport;

    /**
     * @dport: Port on destination that is to receive packet. Must be
     * in the same position as in a TCP header.
     */
    __be16 dport;

    /**
     * @unused1: corresponds to the sequence number field in TCP headers;
     * must not be used by Homa, in case it gets incremented during TCP
     * offload.
     */
    __be32 unused1;

    __be32 unused2;

    /**
     * @doff: High order 4 bits holds the number of 4-byte chunks in a
     * data_header (low-order bits unused). Used only for DATA packets;
     * must be in the same position as the data offset in a TCP header.
     */
    __u8 doff;

    /** @type: One of the values of &enum packet_type. */
    __u8 type;

    __u16 seq;

    /**
     * @checksum: not used by Homa, but must occupy the same bytes as
     * the checksum in a TCP header (TSO may modify this?).*/
    __be16 checksum;

    __u16 unused4;

    /**
     * @sender_id: the identifier of this RPC as used on the sender (i.e.,
     * if the low-order bit is set, then the sender is the server for
     * this RPC).
     */
    __be64 sender_id;
} __attribute__((packed));
#ifdef __cplusplus
static_assert(sizeof(struct common_header) == 28, "common_header size is not 28 bytes");
#else
_Static_assert (sizeof(struct common_header) == 28, "common_header size is not 28 bytes");
#endif

struct unknown_header {
    /** @common: Fields common to all packet types. */
    struct common_header common;
} __attribute__((packed));

struct busy_header {
    /** @common: Fields common to all packet types. */
    struct common_header common;
} __attribute__((packed));

struct need_ack_header {
    /** @common: Fields common to all packet types. */
    struct common_header common;
} __attribute__((packed));

struct homa_ack {
    __be64 rpcid;
    __be16 sport;
    __be16 dport;
} __attribute__((packed));
#ifdef __cplusplus
static_assert(sizeof(struct homa_ack) == 12, "homa_ack size is not 12 bytes");
#else
_Static_assert (sizeof(struct homa_ack) == 12, "homa_ack size is not 12 bytes");
#endif

struct data_segment {
    __be32 offset;

    __be32 segment_length;

    struct homa_ack ack;

    char data[0];
} __attribute__((packed));
#ifdef __cplusplus
static_assert(sizeof(struct data_segment) == 20, "data_segment size is not 20 bytes");
#else
_Static_assert (sizeof(struct data_segment) == 20, "data_segment size is not 20 bytes");
#endif

struct data_header {
    struct common_header common;

    __be32 message_length;

    __be32 incoming;

    __be16 cutoff_version;

    __u8 retransmit;

    __u8 unused1;

    struct data_segment seg;
} __attribute__((packed));
#ifdef __cplusplus
static_assert(sizeof(struct data_header) == 60, "data_header size is not 60 bytes");
#else
_Static_assert (sizeof(struct data_header) == 60, "data_header size is not 60 bytes");
#endif

struct grant_header {
    /** @common: Fields common to all packet types. */
    struct common_header common;

    /**
     * @offset: Byte offset within the message.
     *
     * The sender should now transmit all data up to (but not including)
     * this offset ASAP, if it hasn't already.
     */
    __be32 offset;

    /**
     * @priority: The sender should use this priority level for all future
     * MESSAGE_FRAG packets for this message, until a GRANT is received
     * with higher offset. Larger numbers indicate higher priorities.
     */
    __u8 priority;

    /**
     * @resend_all: Nonzero means that the sender should resend all previously
     * transmitted data, starting at the beginning of the message (assume
     * that no packets have been successfully received).
     */
    __u8 resend_all;
} __attribute__((packed));

struct resend_header {
    /** @common: Fields common to all packet types. */
    struct common_header common;

    /**
     * @offset: Offset within the message of the first byte of data that
     * should be retransmitted.
     */
    __be32 offset;

    /**
     * @length: Number of bytes of data to retransmit; this could specify
     * a range longer than the total message size. Zero is a special case
     * used by servers; in this case, there is no need to actually resend
     * anything; the purpose of this packet is to trigger an UNKNOWN
     * response if the client no longer cares about this RPC.
     */
    __be32 length;

    /**
     * @priority: Packet priority to use.
     *
     * The sender should transmit all the requested data using this
     * priority.
     */
    __u8 priority;
} __attribute__((packed));

#define HOMA_COMMON_H sizeof(struct common_header)
#define HOMA_DATA_H sizeof(struct data_header)
#define HOMA_GRANT_H sizeof(struct grant_header)
#define HOMA_RESEND_H sizeof(struct resend_header)
#define HOMA_UNKNOWN_H sizeof(struct unknown_header)
#define HOMA_BUSY_H sizeof(struct busy_header)