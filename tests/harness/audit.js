window.ztAudit = async function () {
    const wait = ms => new Promise(r => setTimeout(r, ms));
    const report = [];
    const onError = e => report.push('script error: ' + (e.message || e.reason || e));
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onError);
    const still = document.head.appendChild(document.createElement('style'));
    still.textContent = '#zt-app * { animation: none !important; transition: none !important; }';
    const check = label => {
        document.querySelectorAll('#zt-app *').forEach(el => {
            const cs = getComputedStyle(el);
            if (el.clientWidth === 0 || cs.display === 'none' || el.closest('svg')) {
                return;
            }
            const clipped = cs.overflowX !== 'visible' || cs.textOverflow === 'ellipsis';
            if (!clipped && el.scrollWidth > el.clientWidth + 2 && el.children.length === 0) {
                report.push(label + ': text wider than its box: ' + (el.getAttribute('class') || el.tagName) + ' "' +
                            el.textContent.slice(0, 60) + '" ' + el.scrollWidth + '>' + el.clientWidth);
            }
        });
        if (document.documentElement.scrollWidth > document.documentElement.clientWidth + 1) {
            report.push(label + ': the page scrolls sideways (' + document.documentElement.scrollWidth + ')');
        }
        const top = Array.from(document.querySelectorAll('#zt-app .zt-overlay')).pop();
        (top ? top.querySelectorAll('.zt-panel-foot button') : []).forEach(b => {
            const r = b.getBoundingClientRect();
            if (!r.width) {
                return;
            }
            for (const [x, y] of [[r.left + r.width / 2, r.top + r.height / 2], [r.left + 3, r.bottom - 3], [r.right - 3, r.bottom - 3]]) {
                const hit = document.elementFromPoint(x, y);
                if (!hit || (!b.contains(hit) && !hit.contains(b))) {
                    report.push(label + ': button "' + b.textContent.trim() + '" is ' +
                                (hit ? 'covered by ' + (hit.id ? '#' + hit.id : hit.getAttribute('class') || hit.tagName) : 'outside the window'));
                    break;
                }
            }
        });
        document.querySelectorAll('#zt-app .zt-drawer, #zt-app .zt-modal').forEach(panel => {
            panel.querySelectorAll('*').forEach(el => {
                const r = el.getBoundingClientRect();
                const p = panel.getBoundingClientRect();
                if (r.width && (r.right > p.right + 1 || r.left < p.left - 1) && !el.closest('.zt-browser-scroll')) {
                    report.push(label + ': sticks out of its dialog: ' + (el.getAttribute('class') || el.tagName) + ' "' +
                                (el.textContent || '').slice(0, 40) + '"');
                }
            });
        });
    };
    const closeAll = async () => {
        for (let i = 0; i < 4; i++) {
            document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}));
            await wait(80);
        }
    };
    const tab = async name => {
        const t = [...document.querySelectorAll('#zt-app .zt-tab')].find(x => x.textContent.includes(name));
        if (!t) {
            report.push(name + ': no such tab, not checked');
            return;
        }
        t.click();
        await wait(1500);
        check(name);
    };
    const open = async (text, label, ms = 1200) => {
        const b = [...document.querySelectorAll('#zt-app button')]
            .find(x => x.textContent.trim().startsWith(text) && !x.disabled && x.getClientRects().length);
        if (!b) {
            report.push(label + ': no "' + text + '" button, not checked');
            return false;
        }
        b.click();
        await wait(ms);
        check(label);
        return true;
    };

    await tab('Snapshots');
    for (const [text, label] of [['Edit', 'schedule & retention'], ['Add dataset', 'add dataset']]) {
        if (await open(text, label)) {
            await closeAll();
        }
    }
    const more = document.querySelector('#zt-app button[title="More"]');
    if (more) {
        more.click();
        await wait(300);
    }
    if (await open('Show log', 'snapshot log')) {
        await closeAll();
    }
    await tab('Browse');
    const snap = document.querySelector('#zt-app .zt-snap');
    if (snap) {
        snap.click();
        await wait(2500);
        check('snapshot browser');
    }
    if (await open('Restore', 'restore dialog')) {
        if (await open('Into another folder', 'restore into a folder', 600) && await open('Browse...', 'folder picker')) {
            const dir = document.querySelector('#zt-app .zt-overlay:last-child tr.clickable');
            if (dir) {
                dir.click();
                await wait(800);
            }
            await open('New folder', 'folder picker / new folder', 300);
        }
        await closeAll();
    }
    await tab('Replication');
    for (const [text, label] of [['New job', 'new job'], ['Log', 'replication log']]) {
        if (await open(text, label, 1500)) {
            await closeAll();
        }
    }
    await tab('Conversion');
    await tab('Settings');
    if (await open('Show diagnostics', 'diagnostics', 2500)) {
        await closeAll();
    }
    await tab('Snapshots');
    still.remove();
    window.removeEventListener('error', onError);
    window.removeEventListener('unhandledrejection', onError);
    return [...new Set(report)];
};
