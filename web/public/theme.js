// Applies the stored or system theme before the first paint, so dark mode does not flash
// light while the application loads. Keep the storage key, the attribute, and the
// precedence in sync with src/components/workspace/use-theme.ts.
{
  let theme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  try {
    const stored = localStorage.getItem('console-theme')
    if (stored === 'light' || stored === 'dark') theme = stored
  } catch {
    // Without storage access the system preference applies.
  }
  document.documentElement.dataset.theme = theme
}
