# SYSTEM-AWARENESS — status PC + waktu disisipin ke tiap chat

> Owner: Mr.Dev 2026-06-26. Tujuan: agent sadar kondisi PC (spek/OS/CPU/GPU/temp/RAM/load) + WAKTU
> sekarang tiap chat → tau data lama/baru (anti-halu cutoff) + kalau panas bisa nyaranin jeda.

## CARA KERJA
File NON-frozen seam `router/sysstatus_ext.go`: `systemStatusText()` baca live —
- waktu lokal+UTC (time.Now), OS (/proc/sys/kernel/osrelease), CPU model+cores+load (/proc/cpuinfo,
  /proc/loadavg), RAM total+used (/proc/meminfo), GPU name+temp+util (nvidia-smi, cache 30s),
  CPU temp (/sys/class/thermal). Static di-cache; dinamis (temp/load/RAM/waktu) live.
`InjectSystemStatus(req)` prepend 1 system-message `[STATUS_PC] ...` ke SETIAP chat di
`handlers_chat.go` (chatCompletionsHandler, setelah claude-cli bypass). Anti-dobel (cek udah ada).
Multi-OS: detail /proc cuma linux; OS lain → OS+waktu minimal.

## SWITCH GUI
`FLOWORK_SYS_STATUS` (bool, default ON, kategori "Router / Context"). OFF = gak disisipin.

## VERIFIKASI 2026-06-26
Echo-test (suruh model ulangi baris [STATUS_PC]) → balas REAL:
`[STATUS_PC] waktu: 2026-06-26 15:24 WIB (UTC 08:24) | OS: linux 6.17.0-29-generic | CPU: i5-14400F
×16 load 1.71 | RAM: 17.9/63 GB | GPU: RTX 4060 49°C util 2% | CPU 49°C`. Build+TestKernelFreeze PASS.

## HOT → JEDA
Blok nyertain advisory: "kalau GPU/CPU temp >80°C / load berat → hindari kerjaan berat barengan /
sarankan jeda". Agent SADAR (lewat reasoning) — bukan force-sleep. LLM idle-sleep (`llm_idle_sleep.go`)
tetap UTUH terpisah.

## FILE
- `router/sysstatus_ext.go` (FROZEN) — logic + InjectSystemStatus.
- `router/handlers_chat.go` (non-frozen orchestration) — panggil InjectSystemStatus.
- `agent/internal/fwswitch/registry.go` — switch FLOWORK_SYS_STATUS.
