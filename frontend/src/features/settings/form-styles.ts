export const settingsFormClass = `
  grid grid-cols-1 items-end gap-3.5 border-b border-glass-divider px-5 pt-4 pb-4.5  [&_label:not(.check)]:grid
  [&_label:not(.check)]:min-w-0 [&_label:not(.check)]:gap-1.5 [&_label:not(.check)]:text-[11.5px] [&_label:not(.check)]:font-semibold
  [&_label:not(.check)]:text-muted [&_input:not([type=checkbox])]:w-full [&_input:not([type=checkbox])]:rounded-lg
  [&_input:not([type=checkbox])]:border [&_input:not([type=checkbox])]:border-glass-field-edge
  [&_input:not([type=checkbox])]:bg-glass-field [&_input:not([type=checkbox])]:px-3 [&_input:not([type=checkbox])]:py-2
  [&_input:not([type=checkbox])]:text-[13px] [&_input:not([type=checkbox])]:text-text [&_select]:w-full [&_select]:rounded-lg
  [&_select]:border [&_select]:border-glass-field-edge [&_select]:bg-glass-field [&_select]:px-3 [&_select]:py-2
  [&_select]:text-[13px] [&_select]:text-text [&_fieldset]:col-span-full [&_fieldset]:m-0 [&_fieldset]:flex [&_fieldset]:flex-wrap
  [&_fieldset]:gap-2 [&_fieldset]:border-0 [&_fieldset]:p-0 [&_legend]:mb-0.5 [&_legend]:basis-full [&_legend]:text-xs
  [&_legend]:text-muted
`;
