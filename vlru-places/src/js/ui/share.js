import { toast } from './toast.js';
import { itemUrl } from './cards.js';

/** «Поделиться»: системное меню, если есть, иначе копирование ссылки. */
export async function shareLink({ title, text = '', hash }) {
  const url = `${location.origin}${location.pathname}${hash}`;
  if (navigator.share) {
    try {
      await navigator.share({ title, text, url });
      return;
    } catch (error) {
      if (error.name === 'AbortError') return;
      console.warn('Системное меню «Поделиться» недоступно, копируем ссылку', error);
    }
  }
  try {
    await navigator.clipboard.writeText(url);
    toast('Ссылка скопирована', { type: 'success' });
  } catch (error) {
    console.warn('Не удалось скопировать ссылку', error);
    toast(`Ссылка: ${url}`, { duration: 8000 });
  }
}

export function shareItem(item) {
  if (!item) return;
  shareLink({ title: item.title, text: item.short, hash: itemUrl(item) });
}
