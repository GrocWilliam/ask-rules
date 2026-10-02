// wakeLock.ts — Garde l'écran allumé pendant une partie
//
// Le navigateur relâche le verrou quand l'onglet passe en arrière-plan : il
// est redemandé au retour. Sans support (Firefox ancien…), ne fait rien.

let sentinel: WakeLockSentinel | null = null;
let wanted = false;

async function acquire() {
  if (!wanted || sentinel || document.visibilityState !== 'visible') return;
  try {
    sentinel = await navigator.wakeLock.request('screen');
    sentinel.addEventListener('release', () => (sentinel = null));
  } catch {
    // refusé (économie d'énergie…) : sans conséquence
  }
}

function onVisibilityChange() {
  acquire();
}

export function keepScreenOn(on: boolean) {
  if (typeof navigator === 'undefined' || !('wakeLock' in navigator)) return;
  if (on === wanted) return;
  wanted = on;
  if (on) {
    document.addEventListener('visibilitychange', onVisibilityChange);
    acquire();
  } else {
    document.removeEventListener('visibilitychange', onVisibilityChange);
    sentinel?.release().catch(() => {});
    sentinel = null;
  }
}
