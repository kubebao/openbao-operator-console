// downloadText saves generated text, such as reader RBAC manifests, as a local file.
export function downloadText(value: string, filename: string, type = 'application/yaml') {
  const url = URL.createObjectURL(new Blob([value], { type }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  // Some browsers start the download after click() returns, so the URL must outlive it.
  window.setTimeout(() => {
    URL.revokeObjectURL(url)
  }, 1000)
}
