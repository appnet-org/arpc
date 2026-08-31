#include <stdint.h>

#include <intf/intf_ebpf.h>
#include <runtime/tcp.h>

void dctcp_wnd_cc(struct tcp_connection *c, struct bpf_cc_snapshot *stats, uint64_t curr_tsc)
{
    struct cc_dctcp_wnd *cc = &c->cc_data.dctcp_wnd;
    uint64_t ecn_rate, incr;
    uint32_t rtt = stats->rtt, win = cc->window;

    assert(win >= 1448);

    /* If RTT is zero, use estimate */
    if (rtt == 0)
    {
        rtt = TCP_RTT_INIT;
    }

    c->cc_last_rtt = rtt;

    // static int print_freq = 10000;
    // if (print_freq-- <= 0) {
    //     print_freq = 10000;
    //     printf("Connection(%p): RTT: %u us, rate(kbps): %u, %d\n", c, rtt, c->cc_rate, cc->slowstart);
    // }

    /* Slow start */
    if (cc->slowstart)
    {
        if (stats->c_drops == 0 && stats->c_ecnb == 0 && c->cc_rexmits == 0)
        {
            /* double window, but ensure we don't overflow it */
            if (win + stats->c_ackb > win)
                win += stats->c_ackb;
        }
        else
        {
            /* if we see any indication of congestion go into congestion avoidance */
            cc->slowstart = 0;
        }
    }

    /* Congestion avoidance */
    if (!cc->slowstart)
    {
        /* if we have drops, cut by half */
        if (stats->c_drops > 0 || c->cc_rexmits > 0)
        {
            win /= 2;
        }
        else
        {
            /* update ECN rate */
            if (stats->c_ackb > 0)
            {
                stats->c_ecnb = (stats->c_ecnb <= stats->c_ackb ? stats->c_ecnb : stats->c_ackb);
                ecn_rate = (((uint64_t)stats->c_ecnb) * UINT32_MAX) / stats->c_ackb;

                /* EWMA */
                ecn_rate = ((ecn_rate * CC_DCTCP_WEIGHT) +
                            ((uint64_t)cc->ecn_rate *
                             (UINT32_MAX - CC_DCTCP_WEIGHT)));
                ecn_rate /= UINT32_MAX;
                cc->ecn_rate = ecn_rate;
            }

            /* if ecn marks: reduce window */
            if (stats->c_ecnb > 0)
            {
                win = (((uint64_t)win) * (UINT32_MAX - cc->ecn_rate / 2)) / UINT32_MAX;
            }
            else
            {
                /* additive increase */
                assert(win != 0);
                incr = ((uint64_t)stats->c_ackb * 1448) / win;
                if ((uint32_t)(win + incr) > win)
                    win += incr;
            }
        }
    }

    /* Ensure window is at least 1 mss */
    if (win < 1448)
        win = 1448;

    /* A window larger than the send buffer also does not make much sense */
    if (win > c->tx_buf_size)
        win = c->tx_buf_size;

    c->cc_rate = window_to_rate(win, rtt);
    cc->window = win;
    c->cc_rexmits = 0;
}

void dctcp_rate_cc(struct tcp_connection *c, struct bpf_cc_snapshot *stats, uint64_t curr_tsc)
{
    struct cc_dctcp_rate *cc = &c->cc_data.dctcp_rate;
    uint32_t cur_ts = cycles_to_us(curr_tsc); /* convert to us */

    uint64_t ecn_rate;
    uint32_t act_rate, rate = c->cc_rate, rtt = stats->rtt, c_ecnb, c_acks,
                       c_ackb, c_drops;

    if (rtt == 0)
    {
        rtt = TCP_RTT_INIT;
    }

    c->cc_last_rtt = rtt;

    // static int print_freq = 10000;
    // if (print_freq-- <= 0) {
    //     print_freq = 10000;
    //     printf("Connection(%p): RTT: %u us, rate(kbps): %u, %d\n", c, rtt, c->cc_rate, cc->slowstart);
    // }

    c_ecnb = cc->unproc_ecnb + stats->c_ecnb;
    c_acks = cc->unproc_acks + stats->c_acks;
    c_ackb = cc->unproc_ackb + stats->c_ackb;
    c_drops = cc->unproc_drops + stats->c_drops;

    if (c_ackb < CC_DCTCP_MINBYTES)
    {
        cc->unproc_ecnb = c_ecnb;
        cc->unproc_acks = c_acks;
        cc->unproc_ackb = c_ackb;
        cc->unproc_drops = c_drops;
        return;
    }
    else
    {
        cc->unproc_ecnb = 0;
        cc->unproc_acks = 0;
        cc->unproc_ackb = 0;
        cc->unproc_drops = 0;
        (void)c_acks;
    }

    /* calculate actual rate */
    if (cc->last_ts != 0)
    {
        act_rate = c_ackb * 8 * 1000 / (cur_ts - cc->last_ts);
    }
    else
    {
        act_rate = 0;
    }
    // printf("RTT %u us, act_rate(%u), cc->act_rate(%u)\n", rtt, act_rate, cc->act_rate);
    cc->act_rate = (7 * cc->act_rate + act_rate) / 8;
    act_rate = (act_rate >= cc->act_rate ? act_rate : cc->act_rate);

    /* clamp rate to actually used rate * 1.5 */
    if (rate > (uint64_t)act_rate * 15 / 10)
    {
        rate = (uint64_t)act_rate * 15 / 10;
    }

    /* Slow start */
    if (cc->slowstart)
    {
        if (c_drops == 0 && c_ecnb == 0 && c->cc_rexmits == 0)
        {
            /* double rate*/
            if (rate * 2 >= rate)
                rate *= 2;
            else
                rate = UINT32_MAX;
        }
        else
        {
            /* if we see any indication of congestion go into congestion avoidance */
            cc->slowstart = 0;
            // printf("Exit slowstart %u,%u,%u\n", c_drops, c_ecnb, c->cc_rexmits);
        }
    }

    /* Congestion avoidance */
    if (!cc->slowstart)
    {
        /* if we have drops, cut by half */
        if (c_drops > 0 || c->cc_rexmits > 0)
        {
            rate /= 2;
        }
        else
        {
            /* update ECN rate */
            if (c_ackb > 0)
            {
                c_ecnb = (c_ecnb <= c_ackb ? c_ecnb : c_ackb);
                ecn_rate = (((uint64_t)c_ecnb) * UINT32_MAX) / c_ackb;

                /* EWMA */
                ecn_rate = ((ecn_rate * CC_DCTCP_WEIGHT) +
                            ((uint64_t)cc->ecn_rate *
                             (UINT32_MAX - CC_DCTCP_WEIGHT)));
                ecn_rate /= UINT32_MAX;
                cc->ecn_rate = ecn_rate;
            }

            /* if ecn marks: reduce window */
            if (c_ecnb > 0)
            {
                rate = (((uint64_t)rate) * (UINT32_MAX - cc->ecn_rate / 2)) /
                       UINT32_MAX;
            }
            else if (CC_DCTCP_MIMD == 0)
            {
                /* additive increase */
                rate += CC_DCTCP_STEP;
            }
            else
            {
                /* multiplicative increase */
                rate += (((uint64_t)rate) * CC_DCTCP_MIMD) / UINT32_MAX;
            }
        }
    }

    /* ensure we're at least at the minimum rate */
    if (rate < CC_DCTCP_MIN)
        rate = CC_DCTCP_MIN;

    /* re-enter slow-start */
    if (rate == CC_DCTCP_MIN && c_drops == 0 && c_ecnb == 0 && c->cc_rexmits == 0)
        cc->slowstart = 1;

    c->cc_rate = rate;
    cc->last_ts = cur_ts;
    c->cc_rexmits = 0;
}

void timely_cc(struct tcp_connection *c, struct bpf_cc_snapshot *stats, uint64_t curr_tsc)
{
    struct cc_timely *cc = &c->cc_data.timely;
    uint32_t cur_ts = cycles_to_us(curr_tsc); /* convert to us */
    int32_t new_rtt_diff = 0;
    uint32_t new_rtt, new_rate, act_rate;
    uint64_t factor;
    int64_t x, normalized_gradient = 0;

    new_rtt = stats->rtt;
    // static int print_freq = 10000;
    // if (print_freq-- <= 0) {
    //     print_freq = 10000;
    //     printf("Connection(%p): RTT: %u us, rate(kbps): %u\n", c, new_rtt, c->cc_rate);
    // }

    /* calculate actual rate (kbps) */
    if (cc->last_ts != 0)
    {
        act_rate = stats->c_ackb * 8 * 1e3 / (cur_ts - cc->last_ts);
    }
    else
    {
        act_rate = 0;
    }

    cc->act_rate = (7 * cc->act_rate + act_rate) / 8;
    act_rate = (act_rate >= cc->act_rate ? act_rate : cc->act_rate);

    /* no rtt estimate yet, a bit weird */
    if (new_rtt == 0)
        return;

    /* if in slow-start and rtt is above Thigh, exit slow-start */
    if (cc->slowstart && new_rtt > (CC_TIMELY_TLOW + CC_TIMELY_THIGH) / 2)
    {
        cc->slowstart = 0;
    }

    /* re-enter slow-start */
    if (c->cc_rate <= (CC_TIMELY_INIT_RATE << 2) && !cc->slowstart && new_rtt <= (CC_TIMELY_TLOW + CC_TIMELY_THIGH) / 2)
        cc->slowstart = 1;

    /* clamp rate to actually used rate * 1.5 */
    if (!cc->slowstart && c->cc_rate > (uint64_t)act_rate * 15 / 10)
    {
        c->cc_rate = (uint64_t)act_rate * 15 / 10;
    }

    /* can only calculate a gradient if we have a previous rtt */
    if (cc->rtt_prev != 0)
    {
        new_rtt_diff = new_rtt - cc->rtt_prev;

        /* calculate rtt_diff */
        factor = CC_TIMELY_ALPHA / 2;
        x = (INT32_MAX - factor) * cc->rtt_diff + factor * new_rtt_diff;
        cc->rtt_diff = x / INT32_MAX;

        /* calculate normalized_gradient */
        normalized_gradient =
            (int64_t)cc->rtt_diff * INT16_MAX / CC_TIMELY_MIN_RTT;
    }
    cc->rtt_prev = new_rtt;

    uint32_t orig_rate = c->cc_rate;
    if (cc->slowstart)
    {
        c->cc_rate <<= 1;
        if (c->cc_rate > MAX_LINK_BANDWIDTH)
            c->cc_rate = MAX_LINK_BANDWIDTH;
        // printf("slowstart: %.2f Gbps\n", c->cc_rate / 1e6);
    }
    else if (new_rtt < CC_TIMELY_TLOW)
    {
        new_rate = c->cc_rate;
        new_rate += CC_TIMELY_STEP;
        c->cc_rate = new_rate;
        cc->hai_cnt = 0;
        // printf("< CC_TIMELY_TLOW: %.2f Gbps\n", c->cc_rate / 1e6);
    }
    else if (new_rtt > CC_TIMELY_THIGH)
    {
        /* rate *= 1 - beta * (1 - Thigh/rtt)
         * = 1 - a, a = beta * b, b = 1 - d, d = Thigh/rtt */

        uint32_t d = ((uint64_t)UINT32_MAX * CC_TIMELY_THIGH) / new_rtt;
        uint32_t b = UINT32_MAX - d;
        uint32_t a = (((uint64_t)CC_TIMELY_BETA) * b) / UINT32_MAX;
        // uint32_t old_rate = c->cc_rate;
        c->cc_rate = (((uint64_t)c->cc_rate) * (UINT32_MAX - a)) / UINT32_MAX;
        cc->hai_cnt = 0;
        // printf("> CC_TIMELY_THIGH: %.2f --> %.2f Gbps (%u)\n", old_rate / 1e6, c->cc_rate / 1e6, new_rtt);
    }
    else if (normalized_gradient <= 0)
    {
        // uint32_t old_rate = c->cc_rate;
        if (++cc->hai_cnt >= 5)
        {
            c->cc_rate += CC_TIMELY_STEP * 5;
            cc->hai_cnt--;
        }
        else
        {
            c->cc_rate += CC_TIMELY_STEP;
        }
        // printf("normalized_gradient <= 0: %.2f --> %.2f Gbps (%u)\n", old_rate / 1e6, c->cc_rate / 1e6, new_rtt);
    }
    else
    {
        /* rate *= 1 - beta * (normalized_gradient)
         * = 1 - a, a = beta * normalized_gradient */

        int64_t a = ((int64_t)(CC_TIMELY_BETA / 2)) * normalized_gradient;
        int64_t b = a / INT16_MAX;
        int64_t d = (b <= INT32_MAX ? INT32_MAX - b : 0);
        int64_t e = ((int64_t)(uint64_t)c->cc_rate) * d;
        int64_t f = e / INT32_MAX;
        // uint32_t old_rate = c->cc_rate;
        c->cc_rate = f;

        cc->hai_cnt = 0;
        // printf("else: %.2f --> %.2f Gbps (%u)\n", old_rate / 1e6, c->cc_rate / 1e6, new_rtt);
    }

    if (c->cc_rate < orig_rate / 2)
    {
        c->cc_rate = orig_rate / 2;
    }

    if (c->cc_rate < CC_TIMELY_MIN_RATE)
        c->cc_rate = CC_TIMELY_MIN_RATE;

    c->cc_last_rtt = stats->rtt;
    cc->last_ts = cur_ts;
    c->cc_rexmits = 0;

    // printf("RTT: %u us, rate(kbps): %u\n", new_rtt, c->cc_rate);
}