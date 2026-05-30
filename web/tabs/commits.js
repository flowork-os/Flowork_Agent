import { esc, ago, fetchJSON } from '../js/utils.js';

export async function render(mainEl) {
  mainEl.innerHTML = `
    <h2>📋 Progress Kode</h2>
    <div class="sub">Git log untuk melacak perubahan langsung yang dilakukan oleh AI atau Ayah.</div>
    <div class="card">
      <div class="ch">100 Commit Terakhir</div>
      <div class="cb" id="commits"><div class="empty">Running git log...</div></div>
    </div>
  `;

  try {
    const data = await fetchJSON('/api/commits');
    const el = document.getElementById('commits');
    
    if(!data.commits || !data.commits.length) {
      el.innerHTML = '<div class="empty">Tidak ada progress terdeteksi.</div>';
      return;
    }
    
    el.innerHTML = '<table class="tt-table"><thead><tr><th>Waktu</th><th>Author</th><th>Pesan</th><th>Hash</th></tr></thead><tbody>' + 
      data.commits.map(c => `
        <tr>
          <td style="white-space:nowrap;color:var(--text-muted)">${ago(c.date)}</td>
          <td><b>${esc(c.author)}</b></td>
          <td>${esc(c.subject)}</td>
          <td style="font-family:monospace;color:#64748b">${esc(c.hash.substring(0,7))}</td>
        </tr>
      `).join('') + '</tbody></table>';

  } catch(e) {
    document.getElementById('commits').innerHTML = `<div class="err">Error: ${esc(e.message)}</div>`;
  }
}

