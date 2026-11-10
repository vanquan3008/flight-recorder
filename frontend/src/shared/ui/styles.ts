export const secondaryButtonClass = `
  inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border border-glass-field-edge bg-glass-field px-3
  py-1.5 text-xs font-semibold leading-snug text-text transition-colors hover:border-accent/45 hover:bg-glass-field-focus
`;

export const primaryButtonClass = `
  inline-flex items-center justify-center gap-1.5 rounded-lg border border-btn-border bg-btn px-3.5 py-2 text-xs
  font-semibold text-btn-text transition-colors hover:bg-btn-hover
`;

export const toolbarClass = `
  mb-3 flex flex-wrap items-center gap-2 [&_input:not([type=checkbox])]:min-w-[150px] [&_input:not([type=checkbox])]:rounded-lg
  [&_input:not([type=checkbox])]:border [&_input:not([type=checkbox])]:border-glass-field-edge
  [&_input:not([type=checkbox])]:bg-glass-field [&_input:not([type=checkbox])]:px-3 [&_input:not([type=checkbox])]:py-1.5
  [&_input:not([type=checkbox])]:text-[12.5px] [&_input:not([type=checkbox])]:text-text [&_select]:min-w-[150px] [&_select]:rounded-lg
  [&_select]:border [&_select]:border-glass-field-edge [&_select]:bg-glass-field [&_select]:px-3 [&_select]:py-1.5
  [&_select]:text-[12.5px] [&_button:not(.button-secondary)]:inline-flex [&_button:not(.button-secondary)]:items-center
  [&_button:not(.button-secondary)]:gap-1.75 [&_button:not(.button-secondary)]:rounded-lg
  [&_button:not(.button-secondary)]:bg-btn [&_button:not(.button-secondary)]:px-3.5 [&_button:not(.button-secondary)]:py-1.5
  [&_button:not(.button-secondary)]:text-xs [&_button:not(.button-secondary)]:font-semibold
  [&_button:not(.button-secondary)]:text-btn-text max-[620px]:[&>input]:min-w-0 max-[620px]:[&>input]:flex-[1_1_140px]
`;
