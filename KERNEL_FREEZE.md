# KERNEL FREEZE v1 — Flowork Agent

**Frozen:** 2026-06-07 · **Owner:** Aola Sahidin (Mr.Dev)

Ini menandai **microkernel inti "papan kosong"** sebagai BEKU (ditulis sekali,
tidak diedit lagi). File di bawah = ABI + boundary keamanan abadi. Lapis plug-and-play
(handler, GUI, agent, scanner, scanapi, agentmgr, dst) TIDAK beku — boleh berkembang.

Audit pre-freeze lengkap (AUTH/LOKET/TOOLS/KERNEL + lapis-2). Temuan serius (drive-by-localhost RCE,
stored XSS, protector evasion) sudah di-fix sebelum freeze ini.

**Aturan:** mengubah file beku = perlu unfreeze EKSPLISIT owner + regenerate manifest ini.
Guardian lapis-2 (OS-immutable / runtime ARM) BELUM dipasang — itu langkah terpisah.

## SHA256 manifest ( file inti)
```
983fac579c04f03425bbb8b315cfbf711cde4151f264aba955c043acec68d8c7  internal/loket/contract.go
ea3fcf4f52fd23bb05dee97495a4feec9c93a3a40d2d4cb3546803ffe56e484d  internal/loket/dispatcher.go
2a6ac7ff1c4685f5ad9e3156b5027a3a07a987dce10d44763dcdf9f96e46f54b  internal/loket/manifest.go
392d8cebff1f59a10edabd20825677ae411947fe161061cf2501af7e9f8f2422  internal/loket/ratelimit.go
d8fd9fd522099b8fb3a36cfe66425ad81b0c39447d433b727a8a8ffe531fa036  internal/loket/service.go
8400393196d51ab4a672272fe6a0e7b96ef76c4b45d78a7bfae1c40d90b62cd1  internal/loket/store.go
ca92bee50f766cc62f779486f0373ec37b58290d167960eb087d2ebfd4a7ada8  internal/loket/providers.go
c5f9dcc41a7fd12391420c9bdeb9b5b2a8d12c1a0ec9074730eeccdfb296a147  internal/loket/providers_net.go
e6c5ca508d9a039f1b5da6bfab9edf8eb72ec0b49247d8949b1a4919a9a5c85e  internal/loket/providers_syscall.go
98ba5449cb916d9427197d12de85f0eed76c562aca669f24ca49c8be8435f73b  internal/loket/providers_registry.go
64382ed32004214cc90768997f5d2a7941b8ceabaafcd139a59d64cb2d80afcf  internal/loket/providers_schedule.go
ad81bf7598b85bc88f0cc807c7fb25ed393579a43c4e2e5e6e6dae9fb9d83644  internal/loket/providers_gui.go
d6653fdf33889ed50970c166c99a2a6d6b7474f95d8b03d8da26e6315e19e28d  internal/kernel/runtime/runtime.go
945156d6671ec922cae0a2b5517d95f3f4dd50ca872fb3c23c418d9275ace88a  internal/kernel/runtime/host.go
975a6df2febdd9c87353b336c2812e7d15e8d377fda0c161f57e541629e7e24d  internal/kernel/runtime/instance.go
2958bac6b6caa8045632d2ea8e10ee10e170c3cf580325e12729c80d09b85770  internal/kernel/loader/manifest.go
4419347a2ec81cb6fc455f70cd84cf0b1f92f5ca002e0b3d08f9f22139ca177c  internal/kernel/loader/scanner.go
7309dc08861e939dd04e247de8f140f27f9414cf690de5491123e3340aa3441f  internal/kernel/loader/watcher.go
cf30e7dd8c904526c2b4dfb19873f465c40d8c53efc49b79269aebd03983882e  internal/kernel/broker/broker.go
6c76e204a1f772e8e079e99571470c3a0fce003d5e72459908dda0166b23c8da  internal/kernelhost/kernelhost.go
8512dbb43ad58d98ae7ed50f924696e08b8b7079a9741360d320fcdf92e412c6  internal/floworkauth/floworkauth.go
bc678b09b745c556d1dc4f83c3553c64295292ef8fa1d0318922638e00b32b96  internal/floworkauth/handlers.go
7b29b17ebb778f620b0c02fcba1c1a991978d4996bad95b19836ddb057b647b2  internal/tools/sandbox.go
b2e6fd07a7fe03492536e7dff65b613637865da0f3cfdf5f9c911d7a48cbf868  internal/tools/sandbox_v3.go
15fde2de63a55773dcd2f9070eac4f9a634674705e2302125f167da3757e5381  internal/tools/interceptors.go
e20053cd08afe04ccfbe7c0adc4f0cd615bd54d18802338be371286e49e335f3  internal/tools/registry.go
ed74ea173be0dce7e6e38b978a0cccfd0d9edf8e7e80265a597d63c18bf8709a  internal/protector/baseline.go
```
