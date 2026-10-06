export function formError(error: unknown) {
  return error instanceof Error ? error.message : 'The change could not be saved.'
}
