[ AI EXTERNAL ]
                                            │
                                            V
                                         [ MCP ]
                                            │
                                            V
             ┌─────────────────┐    ┌───────────────────────────────────────────┐
[ USER ] ──> │   ENTRY POINT   │ ──>│         AI AGENT : MR.FLOW (Awal)         │
             │ ├─ TELEGRAM     │    │                                           │ ──> [ USER ]
             │ ├─ CLI          │    │  [ PEMROSESAN PESAN ]                     │
             │ ├─ TUI          │    │   ├─ PREFIX      ──> COMMAND REGISTRY     │
             │ └─ QUALITY CTRL │    │   ├─ SLASH CMD   ──> DIREK TANPA LLM      │
             └─────────────────┘    │   └─ PESAN BIASA ──> INJECT SKILL         │
                                    └───────────────────┬───────────────────────┘
                                                        │
                                                        V
             ┌──────────────────────────────────────────────────────────────────┐
             │                            FRAME 1                               │
[ SCEDUL ] ─>│  [TASK CRYPTO]   [TASK SAHAM]       [TASK FOREX]  [TASK HACKING] │
             │  ├─ Analisa Scam ├─ Lap. Keuangan   ├─ Sentimen   ├─ Job Hacker  │
             │  ├─ Komunitas    ├─ Analisa Fund.   ├─ Analisa News └─ Analisa   │
             │  ├─ Analisa Walet├─ Skandal Dirut   ├─ Analisa Tech    Kode      │
             │  ├─ Analisa Tech └─ Analisa Tech    └─ Analisa Fund              │
             │  └─ Analisa Fund                                                 │
             └──────┬────────────────┬──────────────────┬───────────────┬───────┘
                    │                │                  │               │
                    └────────────────┼──────────────────┼───────────────┘
                                     V                  
                        ┌──────────────────────────────┐
                        │ AMBIL KEPUTUSAN BERDASARKAN  │
                        │             DATA             │
                        └──────────────┬───────────────┘
                                       │
                                       V
                        ┌──────────────────────────────┐
                        │  AI AGENT : MR.FLOW (Akhir)  │
                        └──────────────┬───────────────┘
                                       │
                                       V
                                    [ USER ]