/**
 * Выпадающее меню: кнопка с aria-haspopup и aria-controls, список с role="menu".
 * Закрывается по Escape, клику снаружи и выбору пункта. Стрелки перемещают фокус по пунктам.
 */
export function bindMenu(button, menu, { onSelect } = {}) {
  const items = () => Array.from(menu.querySelectorAll('[role="menuitem"]'));

  const open = () => {
    menu.hidden = false;
    button.setAttribute('aria-expanded', 'true');
    items()[0]?.focus();
    document.addEventListener('pointerdown', outside, true);
  };

  const close = ({ focusButton = false } = {}) => {
    if (menu.hidden) return;
    menu.hidden = true;
    button.setAttribute('aria-expanded', 'false');
    document.removeEventListener('pointerdown', outside, true);
    if (focusButton) button.focus();
  };

  const outside = (event) => {
    if (!menu.contains(event.target) && !button.contains(event.target)) close();
  };

  button.addEventListener('click', () => (menu.hidden ? open() : close()));

  menu.addEventListener('keydown', (event) => {
    const list = items();
    const index = list.indexOf(document.activeElement);
    if (event.key === 'Escape') {
      event.preventDefault();
      close({ focusButton: true });
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      list[(index + 1) % list.length]?.focus();
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      list[(index - 1 + list.length) % list.length]?.focus();
    } else if (event.key === 'Tab') {
      close();
    }
  });

  menu.addEventListener('click', (event) => {
    const item = event.target.closest('[role="menuitem"]');
    if (!item) return;
    close();
    onSelect?.(item.dataset.value, item);
  });

  return { open, close, destroy: () => close() };
}
