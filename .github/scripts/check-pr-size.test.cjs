'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const {
  REVIEW_BUDGET_LIMIT,
  evaluatePrSize,
  loadPolicy,
  parsePolicy,
  validatePolicyTransition,
} = require('./check-pr-size.cjs');

const policyPath = path.join(__dirname, '..', 'grandfather-size-exceptions.json');
const workflowPath = path.join(__dirname, '..', 'workflows', 'pr-size-policy.yml');

function dormantPolicy(overrides = {}) {
  return {
    version: 1,
    enforcement: 'dormant',
    limit: REVIEW_BUDGET_LIMIT,
    activation_snapshot: null,
    grandfathered_prs: [],
    ...overrides,
  };
}

function pullRequest(overrides = {}) {
  return {
    number: 3586,
    additions: 200,
    deletions: 200,
    labels: [],
    ...overrides,
  };
}

test('400 is within the dormant review budget', () => {
  const result = evaluatePrSize(pullRequest(), dormantPolicy());

  assert.equal(result.total, 400);
  assert.equal(result.outcome, 'pass');
  assert.equal(result.enforced, false);
});

test('401 is reported but not enforced while the policy is dormant', () => {
  const result = evaluatePrSize(pullRequest({ additions: 401, deletions: 0 }), dormantPolicy());

  assert.equal(result.total, 401);
  assert.equal(result.outcome, 'warning');
  assert.equal(result.enforced, false);
  assert.match(result.message, /dormant/i);
});

test('missing, null, non-integer, and negative API counts fail closed', () => {
  for (const additions of [undefined, null, '400', 400.5, -1]) {
    assert.throws(
      () => evaluatePrSize(pullRequest({ additions }), dormantPolicy()),
      /additions must be a non-negative integer/
    );
  }

  for (const deletions of [undefined, null, '0', 0.5, -1]) {
    assert.throws(
      () => evaluatePrSize(pullRequest({ deletions }), dormantPolicy()),
      /deletions must be a non-negative integer/
    );
  }
});

test('policy is strict, versioned, dormant, and has an empty snapshot', () => {
  const policy = parsePolicy(fs.readFileSync(policyPath, 'utf8'));

  assert.deepEqual(policy, dormantPolicy());
  assert.throws(
    () => parsePolicy(JSON.stringify(dormantPolicy({ unknown: true }))),
    /unexpected keys/
  );
  assert.throws(() => parsePolicy('{}'), /missing keys/);
  assert.throws(() => parsePolicy('{'), /malformed JSON/);
  assert.throws(() => loadPolicy(''), /unreadable or invalid/);
});

test('policy rejects duplicate and invalid grandfather IDs', () => {
  assert.throws(
    () => parsePolicy(JSON.stringify(dormantPolicy({ grandfathered_prs: [7, 7] }))),
    /duplicate PR number/
  );
  assert.throws(
    () => parsePolicy(JSON.stringify(dormantPolicy({ grandfathered_prs: [0] }))),
    /positive integers/
  );
});

test('policy transitions allow only closed or merged grandfather removals after activation', () => {
  const active = {
    version: 1,
    enforcement: 'enforcing',
    limit: REVIEW_BUDGET_LIMIT,
    activation_snapshot: [7, 8],
    grandfathered_prs: [7, 8],
  };

  assert.deepEqual(
    validatePolicyTransition(dormantPolicy(), active, {
      activationPullRequests: [
        { number: 7, state: 'open', labels: ['size:exception'] },
        { number: 8, state: 'open', labels: ['size:exception'] },
      ],
    }),
    { valid: true }
  );
  assert.throws(
    () => validatePolicyTransition(dormantPolicy(), active, { activationPullRequests: [{ number: 7, state: 'closed', labels: ['size:exception'] }] }),
    /not qualified at activation/
  );
  assert.equal(evaluatePrSize(pullRequest({ number: 7, additions: 401, deletions: 0 }), active).enforced, false);
  assert.deepEqual(
    validatePolicyTransition(active, { ...active, grandfathered_prs: [7] }, { closedOrMergedPullRequests: [{ number: 8, state: 'closed' }] }),
    { valid: true }
  );
  assert.throws(
    () => validatePolicyTransition(active, { ...active, grandfathered_prs: [7] }, { closedOrMergedPullRequests: [] }),
    /not proven closed or merged/
  );
  assert.throws(
    () => validatePolicyTransition(active, { ...active, grandfathered_prs: [7, 8, 9] }),
    /post-snapshot additions/
  );
});

test('workflow is trusted, read-only, and never evaluates merge queue or candidate bytes', () => {
  const workflow = fs.readFileSync(workflowPath, 'utf8');

  assert.match(workflow, /pull_request_target:/);
  assert.match(workflow, /types: \[opened, reopened, synchronize, edited, labeled, unlabeled\]/);
  assert.match(workflow, /contents: read/);
  assert.match(workflow, /pull-requests: read/);
  assert.match(workflow, /issues: read/);
  assert.match(workflow, /ref: \$\{\{ github\.event\.repository\.default_branch \}\}/);
  assert.match(workflow, /persist-credentials: false/);
  assert.match(workflow, /sparse-checkout: \|/);
  assert.match(workflow, /github\.rest\.pulls\.get/);
  assert.match(workflow, /Unable to read live PR facts/);
  assert.doesNotMatch(workflow, /merge_group/);
  assert.doesNotMatch(workflow, /refs\/pull/);
  assert.doesNotMatch(workflow, /github\.event\.pull_request\.head/);
});
