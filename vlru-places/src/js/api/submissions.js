/**
 * Предложения от пользователей: новое место, событие или правка существующего.
 * Публикация только после проверки редакцией: статус 'review' → 'published' | 'rejected'.
 * Без входа отправить тоже можно — тогда контакт для связи обязателен.
 */

import { delay } from './http.js';
import { store } from '../core/store.js';

export async function submitProposal(data) {
  await delay(700);
  const submission = {
    id: `s-${Date.now().toString(36)}`,
    status: 'review',
    createdAt: new Date().toISOString(),
    ...data,
    photos: (data.photos || []).map((file) => file.name),
  };
  store.set((state) => ({ submissions: [submission, ...state.submissions] }));
  return submission;
}
