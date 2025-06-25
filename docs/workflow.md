# Symphony Workflow

This document outlines the end-to-end message processing pipeline in Symphony, covering both the **sender-side** and **receiver-side** workflows. The focus is on how transport elements interact with fragmentation and how the message flows through the custom RPC stack.

---

## Sender-Side Workflow

```
+--------------------+
| Original RPC Msg   |
+--------------------+
           |
           v
+--------------------------+
| Apply Transport Elements |
| (e.g., reliability,      |
|  encryption, compression)|
+--------------------------+
           |
           v
+------------------+
| Fragmentation     |  <-- MTU-aware
+------------------+
           |
           v
+------------------+
| Network Transmission |
+------------------+
```

### Notes

* Transport elements operate on **whole messages**.
* Fragmentation occurs *after* all transport logic to ensure correctness.
* Fragmentation produces MTU-sized packets tagged with sequence/message IDs.

---

## Receiver-Side Workflow

```
+------------------+
| Incoming Fragments |
+------------------+
           |
           v
+------------------+
| Fragment Reassembly |
+------------------+
           |
           v
+--------------------------+
| Apply Transport Elements |
| (e.g., decryption,       |
|  reliability, ordering)  |
+--------------------------+
           |
           v
+--------------------------+
| Apply ANFs               |
| (e.g., logging, firewall)|
+--------------------------+
           |
           v
+------------------+
| Deliver to App   |
+------------------+
```

### Notes

* Reassembly happens *before* transport element processing.
* Transport elements expect a complete message.
* ANFs operate after transport layer to ensure semantic correctness.