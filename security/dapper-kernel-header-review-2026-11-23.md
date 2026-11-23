# Dapper kernel-header VEX review, 2026-11-23

## Exact scope and retained evidence

This is an applicability review of the exact disposable-build package
`pkg:deb/ubuntu/linux-libc-dev@7.0.0-43.43?arch=amd64&distro=ubuntu-26.04`.
It does not qualify a host kernel, a kernel module, or the independently scanned
Catalog binaries. A package-name match is not evidence that its vulnerable kernel
implementation is present. No executable-code finding is excepted here.

- Source producer: `23a64e929664143a6286385e63cf640cd0237c8b`.
- Original [Security release gate 45097702884](https://github.com/PastureStack/catalog-service/actions/runs/45097702884) built and tested the producer; its retained raw scan stopped at the previous raw/VEX mismatch.
- `dapper-rootfs-raw.json`: **3 CRITICAL + 51 HIGH**, all 54 unique CVE/PURL pairs below; 396 total findings and zero secrets remain raw evidence.
  Raw SHA256: `a604912e2c9bf18475d75e202bed55d4029e6900901004274539977aee8d33da`.
- `source-security.json`: zero vulnerabilities and secrets; SHA256
  `68b24bb4264829c8da96df20a29219460cb4b121a3e61f4f205d41f9fe3bb3d8`.
- `product-security.json`: zero vulnerabilities and secrets across both binaries; SHA256
  `1a5fcc99ff0f8a81f9430c7d07f04c659212e4edf19c7cd7b4e8fcbf0f9733d3`.
- `dapper-dpkg.tsv` records `linux-libc-dev:amd64 7.0.0-43.43` and no installed `linux-image`/`linux-modules` package.
- `dapper-vex-code-review.txt` verifies the separate retired-Python-installer boundary; it is not kernel-header package-content proof.
- The original run stopped before its later native package-content assertions. Those assertions are unchanged and must pass in the next official producer gate; this review does not claim they already ran.

## Package-content and source evidence

The exact [Ubuntu Security package](https://security.ubuntu.com/ubuntu/pool/main/l/linux/linux-libc-dev_7.0.0-43.43_amd64.deb)
and [Launchpad package](https://launchpad.net/ubuntu/+archive/primary/+files/linux-libc-dev_7.0.0-43.43_amd64.deb)
were downloaded independently and were byte-identical. Native `dpkg-deb` reports
`Package: linux-libc-dev`, `Version: 7.0.0-43.43`, `Architecture: amd64`.
The 1,885,328-byte Debian archive has SHA256 `3a3adc14e3332fc017b1d0e4e39655298a2a977d51db6e80de533ac60137b07f`.
Its 1,041 regular files are all under `/usr/include/`, the package documentation
directory, or its lintian override. None is ELF. The affected private headers
`include/linux/bpf_verifier.h`, `include/drm/drm_exec.h`, and
`include/sound/hwdep.h` do not appear in the exported package. The other affected
implementation and self-test paths below are likewise absent.

Each CVE was checked against its Linux CNA record in the immutable
[CVEProject snapshot `125285adce6be21198797d6e384cd23cf1094609`](https://github.com/CVEProject/cvelistV5/tree/125285adce6be21198797d6e384cd23cf1094609).
The records identify Linux as the vendor/product and enumerate the affected
`programFiles`. No record below identifies an `include/uapi` or architecture UAPI
path. Private kernel headers are not interchangeable with exported userspace
headers having similar subsystem names. The retained review evidence includes
the original CNA JSON, each JSON SHA256, the package file inventory, and the
unchanged original scanner reports.

All 54 entries qualify as `not_affected` / `vulnerable_code_not_present` **for
this exact package only** because the affected implementations are absent.
CNA programFiles and descriptions establish the implementation location; native
package contents establish its absence. This is not a claim that the kernel CVEs
are fixed, unreachable in every environment, or absent from the runner host.

## Per-CVE review

Every row has the same narrow disposition above. The issue and complete affected
file list give the item-specific reason; the source link preserves the primary
Linux CNA evidence rather than relying on a scanner title alone.

| CVE | Raw severity | Kernel implementation issue | Affected implementation/test files (absent from package) | Linux CNA source |
| --- | --- | --- | --- | --- |
| CVE-2022-50240 | HIGH | android: binder: stop saving a pointer to the VMA | `drivers/android/binder_alloc.c`<br>`drivers/android/binder_alloc.h`<br>`drivers/android/binder_alloc_selftest.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2022/50xxx/CVE-2022-50240.json) |
| CVE-2025-40190 | HIGH | ext4: guard against EA inode refcount underflow in xattr update | `fs/ext4/xattr.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2025/40xxx/CVE-2025-40190.json) |
| CVE-2026-100081 | HIGH | xen-netback: defer xenvif_mcast_addr_list_free() | `drivers/net/xen-netback/interface.c`<br>`drivers/net/xen-netback/xenbus.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/100xxx/CVE-2026-100081.json) |
| CVE-2026-68103 | HIGH | drm/amdgpu: reject mapping a reserved doorbell to a new queue | `drivers/gpu/drm/amd/amdgpu/amdgpu_userq.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/68xxx/CVE-2026-68103.json) |
| CVE-2026-72198 | HIGH | ntfs: reject non-resident records for resident-only attributes | `fs/ntfs/attrib.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/72xxx/CVE-2026-72198.json) |
| CVE-2026-74578 | HIGH | crypto: algif_skcipher - force synchronous processing on trees without ctx->state | `crypto/algif_skcipher.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/74xxx/CVE-2026-74578.json) |
| CVE-2026-89805 | HIGH | drm/pagemap: Fix folio allocation fallback and use-after-put | `drivers/gpu/drm/drm_pagemap.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/89xxx/CVE-2026-89805.json) |
| CVE-2026-98376 | HIGH | bpf: Use array_map_meta_equal for percpu array inner map replacement | `kernel/bpf/arraymap.c`<br>`tools/testing/selftests/bpf/prog_tests/percpu_array_inner_map.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/98xxx/CVE-2026-98376.json) |
| CVE-2026-98420 | HIGH | ksmbd: follow SMB2 session expiration semantics | `fs/smb/server/connection.c`<br>`fs/smb/server/connection.h`<br>`fs/smb/server/mgmt/user_session.c`<br>`fs/smb/server/mgmt/user_session.h`<br>`fs/smb/server/proc.c`<br>`fs/smb/server/server.c`<br>`fs/smb/server/smb2pdu.c`<br>`fs/smb/server/smb2pdu.h`<br>`fs/smb/server/stats.h` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/98xxx/CVE-2026-98420.json) |
| CVE-2026-98869 | HIGH | bpf: Infer zext_dst based on static register liveness analysis | `include/linux/bpf_verifier.h`<br>`kernel/bpf/fixups.c`<br>`kernel/bpf/liveness.c`<br>`kernel/bpf/verifier.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/98xxx/CVE-2026-98869.json) |
| CVE-2026-98890 | HIGH | wifi: libertas: reject short monitor TX frames | `drivers/net/wireless/marvell/libertas/tx.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/98xxx/CVE-2026-98890.json) |
| CVE-2026-99083 | HIGH | ksmbd: fix heap out-of-bounds write in krb5_authenticate() | `fs/smb/server/smb2pdu.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99083.json) |
| CVE-2026-99109 | HIGH | smb: client: reject SetEA requests that do not fit the request buffer | `fs/smb/client/cifssmb.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99109.json) |
| CVE-2026-99110 | HIGH | smb: client: fix heap overflow in cifs_do_set_acl() | `fs/smb/client/cifssmb.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99110.json) |
| CVE-2026-99116 | HIGH | net: psp: do not inherit the Rx association on clone | `net/core/sock.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99116.json) |
| CVE-2026-99117 | HIGH | net: bridge: mcast: don't truncate the port group walk on teardown | `net/bridge/br_multicast.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99117.json) |
| CVE-2026-99126 | HIGH | s390/ctcm: Prevent XID null dereference | `drivers/s390/net/ctcm_mpc.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99126.json) |
| CVE-2026-99128 | HIGH | sctp: validate chunk length in the inqueue parser | `net/sctp/inqueue.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99128.json) |
| CVE-2026-99138 | HIGH | tcp: fix use-after-free in do_tcp_getsockopt(TCP_CC_INFO) | `net/ipv4/tcp.c`<br>`net/ipv4/tcp_dctcp.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99138.json) |
| CVE-2026-99147 | HIGH | drm/exec: don't immediately add prelocked obj to array of locked objs | `drivers/gpu/drm/drm_exec.c`<br>`include/drm/drm_exec.h` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99147.json) |
| CVE-2026-99152 | HIGH | scsi: libiscsi: Validate the task named by a management response | `drivers/scsi/libiscsi.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99152.json) |
| CVE-2026-99153 | HIGH | scsi: libiscsi: Validate the ITT reflected in a Reject PDU | `drivers/scsi/libiscsi.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99153.json) |
| CVE-2026-99154 | HIGH | scsi: libiscsi: Reject a negative task index from parse_pdu_itt | `drivers/scsi/libiscsi.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99154.json) |
| CVE-2026-99164 | HIGH | spi: imx: fix slab corruption from undersized DMA bounce buffers | `drivers/spi/spi-imx.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99164.json) |
| CVE-2026-99166 | HIGH | net: rds: fix uninitialized trans dereference in CM event handler | `net/rds/rdma_transport.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99166.json) |
| CVE-2026-99167 | HIGH | net/sched: fix use-after-free in __tcf_action_put() | `net/sched/act_api.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99167.json) |
| CVE-2026-99185 | HIGH | firewire: core: fix race condition in split transaction handling | `drivers/firewire/core-transaction.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99185.json) |
| CVE-2026-99203 | HIGH | pipe: preserve existing fasync readers on writer setup failure | `fs/pipe.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99203.json) |
| CVE-2026-99212 | HIGH | net/rds: don't let the rdma_cm destroy an id RDS still owns on route failure | `net/rds/rdma_transport.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99212.json) |
| CVE-2026-99213 | HIGH | vsock: treat TCP_CLOSING as once-established | `net/vmw_vsock/af_vsock.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99213.json) |
| CVE-2026-99218 | HIGH | coredump: hold RCU while releasing parked threads | `fs/coredump.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99218.json) |
| CVE-2026-99227 | HIGH | io_uring/waitid: prevent cancel reference wrap | `io_uring/waitid.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99227.json) |
| CVE-2026-99231 | HIGH | io_uring/waitid: own the request before clearing ->head | `io_uring/waitid.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99231.json) |
| CVE-2026-99236 | HIGH | tracing: Reject incomplete event filter expressions | `kernel/trace/trace_events_filter.c`<br>`tools/testing/selftests/ftrace/test.d/filter/event-filter-incomplete.tc` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99236.json) |
| CVE-2026-99239 | HIGH | Bluetooth: L2CAP: refuse __l2cap_chan_add if chan already has conn | `net/bluetooth/l2cap_core.c`<br>`net/bluetooth/l2cap_sock.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99239.json) |
| CVE-2026-99260 | HIGH | ntfs: fix resident conversion in ntfs_new_attr_flags | `fs/ntfs/ea.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99260.json) |
| CVE-2026-99270 | HIGH | tcp: Fix potential UAF in reqsk_timer_handler(). | `net/ipv4/inet_connection_sock.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99270.json) |
| CVE-2026-99272 | HIGH | smb: client: validate the data area in SMB2_read() | `fs/smb/client/smb2pdu.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99272.json) |
| CVE-2026-99274 | HIGH | smb: client: fix fiemap wild read on empty truncated response | `fs/smb/client/smb2ops.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99274.json) |
| CVE-2026-99275 | HIGH | smb: client: reject short SESSION_SETUP responses | `fs/smb/client/smb2misc.c`<br>`fs/smb/client/smb2pdu.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99275.json) |
| CVE-2026-99282 | HIGH | net/sched: sch_teql: fix use-after-free on lower qdisc drop | `net/sched/sch_teql.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99282.json) |
| CVE-2026-99284 | HIGH | net: skbuff: propagate shared-frag marker in skb_zerocopy() | `net/core/skbuff.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99284.json) |
| CVE-2026-99287 | CRITICAL | xen-netback: Zero out sring pointers on unmap | `drivers/net/xen-netback/netback.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99287.json) |
| CVE-2026-99288 | HIGH | inet: reject unhashed sockets during established lookups | `net/ipv4/inet_connection_sock.c`<br>`net/ipv4/inet_hashtables.c`<br>`net/ipv4/syncookies.c`<br>`net/ipv4/tcp_input.c`<br>`net/ipv6/inet6_hashtables.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99288.json) |
| CVE-2026-99290 | HIGH | ksmbd: fix use-after-free after a failed durable reconnect | `fs/smb/server/smb2pdu.c`<br>`fs/smb/server/vfs_cache.c`<br>`fs/smb/server/vfs_cache.h` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99290.json) |
| CVE-2026-99292 | HIGH | smb: client: fix slab-out-of-bounds write in id_mode_to_cifs_acl() | `fs/smb/client/cifsacl.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99292.json) |
| CVE-2026-99295 | HIGH | smb: client: stop processing a read response after a receive error | `fs/smb/client/transport.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99295.json) |
| CVE-2026-99302 | HIGH | nvmet-rdma: reject short connection requests | `drivers/nvme/target/rdma.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99302.json) |
| CVE-2026-99304 | HIGH | ceph: fix integer overflow in the snap trace bounds check | `fs/ceph/snap.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99304.json) |
| CVE-2026-99306 | CRITICAL | ceph: reject mdsmaps that claim more than CEPH_MAX_MDS ranks | `fs/ceph/mdsmap.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99306.json) |
| CVE-2026-99310 | CRITICAL | nfsd: hold cl_lock in client_has_openowners() | `fs/nfsd/nfs4state.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99310.json) |
| CVE-2026-99311 | HIGH | udf: fix return value for non-recorded-allocated extents in udf_map_block() | `fs/udf/inode.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99311.json) |
| CVE-2026-99312 | HIGH | KVM: x86: Unconditionally reset MMU context when emulating INIT | `arch/x86/kvm/x86.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99312.json) |
| CVE-2026-99317 | HIGH | ALSA: hwdep: Serialize DSP load requests | `include/sound/hwdep.h`<br>`sound/core/hwdep.c` | [record](https://raw.githubusercontent.com/CVEProject/cvelistV5/125285adce6be21198797d6e384cd23cf1094609/cves/2026/99xxx/CVE-2026-99317.json) |

## Gate behavior retained

The scanner pin is aligned with the Server workflow:
`ghcr.io/aquasecurity/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb`.
Raw source, product, image-metadata, and exported-rootfs reports remain separate.
Exact raw critical/high CVE+PURL equality, statement uniqueness, native package
identity/content checks, no-kernel-package checks, zero applicable critical/high,
and zero secrets are unchanged. Product and source vulnerability thresholds are
unchanged. Medium/low build findings remain visible and are not converted into
these exceptions. A changed package, new executable-code finding, or changed raw
critical/high set must fail the same equality gate and receive another review.

No already-passed local functional or race test is repeated for this data-only
change. A new official main-producer security gate, using the pinned scanner and
its actual newly exported filesystem, is still required before release.
