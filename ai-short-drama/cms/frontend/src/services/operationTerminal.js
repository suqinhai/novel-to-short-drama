export const terminalOperationStatuses = new Set(['completed', 'partially_failed', 'failed', 'cancelled', 'needs_review'])
export const reviewOperationStatuses = new Set(['needs_review'])
export const failedOperationStatuses = new Set(['partially_failed', 'failed', 'cancelled'])

export function isTerminalOperation(operation) {
  return Boolean(operation?.operation_id && terminalOperationStatuses.has(operation.status))
}

export function isReviewOperation(operation) {
  return Boolean(operation?.operation_id && reviewOperationStatuses.has(operation.status))
}

export function isFailedOperation(operation) {
  return Boolean(operation?.operation_id && failedOperationStatuses.has(operation.status))
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
