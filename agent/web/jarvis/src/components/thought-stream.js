// ── 思考流背景（固定區域 + 滾動 + 漸層消失） ──

const thoughts = [
  'ANALYZING USER INTENT... choosing best response strategy',
  'PARSING CONTEXT WINDOW: 128K TOKENS LOADED',
  'SEARCHING MEMORY... memory/2026-02-12.md',
  'EVALUATING RESPONSE CANDIDATES...',
  'CHECKING SOUL.md personality coherence...',
  'AUDIO FREQUENCY ANALYSIS: PEAK AT 440HZ',
  'CORRELATING long-term & short-term memory',
  'GSAP.TIMELINE({SMOOTHNESS: 0.85})',
  'REASONING CHAIN: premise → analysis → conclusion → verify',
  'SCANNING SKILL REGISTRY: 6 MODULES ACTIVE',
  'SEMANTIC PARSING... confidence 0.94',
  'THREE.VECTOR3 → ANOMALY.POSITION.UPDATE()',
  'SENTIMENT: positive 0.7 / neutral 0.2 / negative 0.1',
  'HEARTBEAT.CHECK() — ALL SYSTEMS NOMINAL',
  'STRUCTURING RESPONSE... ~3 paragraphs',
  'CONTEXT.RELEVANCE.SCORE = 0.89',
  'REVIEWING last 5 messages for coherence',
  'MODEL.INFERENCE({TEMP: 0.7, TOP_P: 0.95})',
  'ASSESSMENT: this needs a technical answer',
  'LOADING AGENT.JSON — CONFIG PASSED',
  'FINDING the best phrasing...',
  'NEURAL.PATHWAY("CREATIVE_REASONING")',
  'CROSS-CHECK: memory × context × instruction',
  'OPTIMIZER.RUN({BEAM_SEARCH, WIDTH: 4})',
  'INTERESTING IDEA... expanding deeper',
  'SYSTEM.COHERENCE.CHECK — PASSED ✓',
  'BALANCING efficiency vs completeness...',
  'TOKEN.BUDGET.REMAINING: 45,231',
  'NEW PATTERN OBSERVED... logging to long-term memory',
  'ATTENTION.LAYER[32].REDISTRIBUTING...',
];

let container = null;
let textArea = null;
let intervalId = null;
let lineIndex = 0;
let currentLine = null;
let charIdx = 0;
let typeTimer = null;

function createContainer() {
  container = document.createElement('div');
  container.id = 'thought-stream';
  container.style.cssText = `
    position: fixed;
    bottom: 30px;
    left: 50%;
    transform: translateX(-50%);
    width: 45%;
    max-width: 600px;
    height: 250px;
    pointer-events: none;
    z-index: 2;
    overflow: hidden;
    -webkit-mask-image: linear-gradient(to bottom, transparent 0%, rgba(0,0,0,0.3) 30%, rgba(0,0,0,0.8) 70%, rgba(0,0,0,1) 100%);
    mask-image: linear-gradient(to bottom, transparent 0%, rgba(0,0,0,0.3) 30%, rgba(0,0,0,0.8) 70%, rgba(0,0,0,1) 100%);
  `;

  textArea = document.createElement('div');
  textArea.style.cssText = `
    position: absolute;
    bottom: 0;
    left: 0;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 0 10px;
  `;

  container.appendChild(textArea);
  document.body.appendChild(container);
}

function startNewLine() {
  const text = thoughts[lineIndex % thoughts.length];
  lineIndex++;

  const line = document.createElement('div');
  line.style.cssText = `
    font-family: "TheGoodMonolith", monospace;
    font-size: 14px;
    letter-spacing: 1.5px;
    line-height: 1.8;
    color: rgba(var(--accent-rgb), 0.45);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    text-align: center;
  `;
  line.textContent = '';
  // 新行加到底部，舊的往上推然後頂部漸層消失
  textArea.appendChild(line);

  // 限制行數，移除最頂部（最舊）的
  while (textArea.children.length > 12) {
    textArea.removeChild(textArea.firstChild);
  }

  // 逐字打出
  currentLine = { el: line, text, charIdx: 0 };
  if (typeTimer) clearInterval(typeTimer);
  typeTimer = setInterval(() => {
    if (!currentLine) return;
    if (currentLine.charIdx < currentLine.text.length) {
      currentLine.el.textContent = currentLine.text.substring(0, currentLine.charIdx + 1);
      currentLine.charIdx++;
    } else {
      clearInterval(typeTimer);
      typeTimer = null;
      currentLine = null;
    }
  }, 40);
}

export function initThoughtStream() {
  createContainer();
  // 延遲啟動
  setTimeout(() => {
    startNewLine();
    intervalId = setInterval(startNewLine, 3500);
  }, 4000);
}

export function stopThoughtStream() {
  if (intervalId) clearInterval(intervalId);
  if (typeTimer) clearInterval(typeTimer);
}
