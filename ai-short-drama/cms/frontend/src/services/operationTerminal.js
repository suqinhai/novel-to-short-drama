export const terminalOperationStatuses = new Set(['completed', 'partially_failed', 'failed', 'cancelled', 'needs_review'])

export function isTerminalOperation(operation) {
  return Boolean(operation?.operation_id && terminalOperationStatuses.has(operation.status))
}

export function createTerminalNotifier(notify) {
  const emittedOperationIds = new Set()
  return (operation) => {
    if (!isTerminalOperation(operation) || emittedOperationIds.has(operation.operation_id)) return false
    emittedOperationIds.add(operation.operation_id)
    notify(operation)
    return true
  }
}
