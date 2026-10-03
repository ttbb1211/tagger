import type {Track} from '@/types';

/**
 * 播放队列 = 「点击试听时曲库列表里可见的那一批曲目（按显示顺序）」。
 * 单曲目录 / 整轨 CUE 专辑都走同一条队列，只是队列内容不同。
 *
 * 两条路径的「播完」信号不一样，但都汇到同一个「取下一首」：
 * - 普通曲目 / 整轨 WAV（后端已切段）→ <audio> 的 ended 事件
 * - 整轨 FLAC 等（前端按 cue 偏移 seek）→ onTimeUpdate 到段尾
 */

/** 播放模式：顺序播放 → 列表循环 → 单曲循环 → 随机播放，由一个按钮循环切换 */
export type PlayerMode = 'order' | 'repeat-all' | 'repeat-one' | 'shuffle';

export const PLAYER_MODES: PlayerMode[] = ['order', 'repeat-all', 'repeat-one', 'shuffle'];

/** 模式按钮点一下切到下一档，循环回第一档 */
export function nextPlayerMode(mode: PlayerMode): PlayerMode {
  const index = PLAYER_MODES.indexOf(mode);
  return PLAYER_MODES[(index + 1) % PLAYER_MODES.length];
}

/** 队列中的下一首；到队尾（或当前曲目不在队列里）返回 null，不环绕。 */
export function nextInQueue(queue: Track[], current: Track | null): Track | null {
  if (!current || queue.length === 0) return null;
  const index = queue.findIndex((item) => item.id === current.id);
  if (index < 0 || index >= queue.length - 1) return null;
  return queue[index + 1];
}

/** 队列中的上一首；到队首（或当前曲目不在队列里）返回 null，不环绕。 */
export function previousInQueue(queue: Track[], current: Track | null): Track | null {
  if (!current || queue.length === 0) return null;
  const index = queue.findIndex((item) => item.id === current.id);
  if (index <= 0) return null;
  return queue[index - 1];
}

/**
 * 随机播放顺序：当前曲目放首位，其余 Fisher-Yates 洗牌。
 *
 * 用「洗好的固定顺序」而不是「每首现抽」——一轮之内不会重复、也不会漏掉曲目，
 * 且将来加「上一首」时语义仍然成立。
 *
 * `random` 可注入（默认 `Math.random`），便于测试取确定序列。
 */
export function shuffleQueue(queue: Track[], current: Track | null, random: () => number = Math.random): Track[] {
  const head = current && queue.some((item) => item.id === current.id) ? current : null;
  const rest = head ? queue.filter((item) => item.id !== head.id) : [...queue];
  for (let i = rest.length - 1; i > 0; i -= 1) {
    const j = Math.floor(random() * (i + 1));
    [rest[i], rest[j]] = [rest[j], rest[i]];
  }
  return head ? [head, ...rest] : rest;
}
