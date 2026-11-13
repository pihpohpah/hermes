import { raw } from '../core/dom.js';

const SPRITE = 'assets/icons.svg';

/**
 * Иконка из спрайта assets/icons.svg (набор Lucide).
 * Декоративная по умолчанию; смысл всегда передаётся текстом рядом или aria-label на кнопке.
 */
export function icon(name, className = '') {
  return raw(
    `<svg class="icon${className ? ` ${className}` : ''}" aria-hidden="true" focusable="false"><use href="${SPRITE}#i-${name}"></use></svg>`,
  );
}
