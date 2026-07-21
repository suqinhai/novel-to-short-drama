import assert from 'node:assert/strict'
import test from 'node:test'
import { createTerminalNotifier, isTerminalOperation } from '../src/services/operationTerminal.js'

test('recognizes only terminal operation states with an operation id', () => {
  assert.equal(isTerminalOperation({ operation_id: 'op_1', status: 'completed' }), true)
  assert.equal(isTerminalOperation({ operation_id: 'op_1', status: 'running' }), false)
  assert.equal(isTerminalOperation({ status: 'failed' }), false)
})

test('notifies once when an operation is already terminal on first render', () => {
  const received = []
  const notify = createTerminalNotifier((operation) => received.push(operation.operation_id))
  const operation = { operation_id: 'op_terminal', status: 'completed' }

  assert.equal(notify(operation), true)
  assert.equal(notify(operation), false)
  assert.deepEqual(received, ['op_terminal'])
})

test('notifies different terminal operations independently', () => {
  const received = []
  const notify = createTerminalNotifier((operation) => received.push(operation.operation_id))

  notify({ operation_id: 'op_1', status: 'failed' })
  notify({ operation_id: 'op_2', status: 'needs_review' })
  assert.deepEqual(received, ['op_1', 'op_2'])
})
