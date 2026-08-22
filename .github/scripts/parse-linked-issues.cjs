'use strict';

// Single parse seam for every pr-check.yml gate reading linked issues.
// Result: { references: [{ number, kind }], errors: [{ raw, reason }] };
// any error fails the workflow closed rather than silently ignoring
// malformed, cross-repository, or ambiguous references.

const CLOSING_KEYWORDS = 'closes|fixes|resolves';
const NON_CLOSING_KEYWORDS = 'refs';
const KEYWORDS = `${CLOSING_KEYWORDS}|${NON_CLOSING_KEYWORDS}`;

// Valid same-repo reference: keyword then #<number>; the trailing (?!\w)
// rejects word-like suffixes (#12foo, #12_bar) as malformed, not #12.
const REFERENCE_PATTERN = new RegExp(`\\b(${KEYWORDS})\\s+#(\\d+)(?!\\w)`, 'gi');

// owner/repo#N fails closed: the approval gate only resolves base-repo
// issues. The portions around `/` and `#` have non-overlapping character
// classes, which bounds each keyword match to a linear scan of one token.
const CROSS_REPO_PATTERN = new RegExp(
  `\\b(${KEYWORDS})\\s+[^\\s#\\/]*\\/[^\\s#]*#\\S*`,
  'gi'
);

// Keyword + `#` without a number is an intended-but-malformed reference;
// the second alternative catches #<digits> with a word-like suffix, while
// [a-zA-Z_] keeps a trailing digit (a valid #1770) from becoming a suffix.
const MALFORMED_PATTERN = new RegExp(
  `\\b(${KEYWORDS})\\s+#(?:(?!\\d)\\S*|\\d+[a-zA-Z_]\\S*)`,
  'gi'
);

// An HTML comment runs to the next `-->` or to the end of the body when
// unclosed, matching GitHub's rendering: everything after an unclosed
// `<!--` is invisible to reviewers and must not count.
const HTML_COMMENT_PATTERN = /<!--[\s\S]*?(?:-->|$)/g;

// Markdown reference definitions, including an optional indented title line,
// are not rendered as reviewer-visible text and must not satisfy this parser.
const MARKDOWN_REFERENCE_DEFINITION_PATTERN =
  /^[ \t]{0,3}\[[^\]\r\n]+\]:[^\r\n]*(?:\r?\n[ \t]+(?:"[^\r\n]*"|'[^\r\n]*'|\([^\r\n]*\)))?/gm;

// Removes non-visible Markdown constructs so only reviewer-visible text remains.
function stripHtmlComments(body) {
  return (body || '')
    .replace(HTML_COMMENT_PATTERN, '')
    .replace(MARKDOWN_REFERENCE_DEFINITION_PATTERN, '');
}

function kindFor(keyword) {
  return NON_CLOSING_KEYWORDS.split('|').includes(keyword.toLowerCase())
    ? 'non-closing'
    : 'closing';
}

function parseLinkedIssues(body) {
  const visible = stripHtmlComments(body);
  const references = [];
  const errors = [];

  for (const match of visible.matchAll(REFERENCE_PATTERN)) {
    references.push({ number: parseInt(match[2], 10), kind: kindFor(match[1]) });
  }

  for (const match of visible.matchAll(CROSS_REPO_PATTERN)) {
    errors.push({
      raw: match[0],
      reason:
        'cross-repository keyword reference; the approval gate only resolves issues in the base repository',
    });
  }

  for (const match of visible.matchAll(MALFORMED_PATTERN)) {
    errors.push({
      raw: match[0],
      reason: `malformed issue reference: expected "#<number>" after "${match[1]}"`,
    });
  }

  const kindsByNumber = new Map();
  for (const reference of references) {
    const kinds = kindsByNumber.get(reference.number) || new Set();
    kinds.add(reference.kind);
    kindsByNumber.set(reference.number, kinds);
  }
  for (const [number, kinds] of kindsByNumber) {
    if (kinds.size > 1) {
      errors.push({
        raw: `#${number}`,
        reason: `ambiguous: issue #${number} is referenced as both closing and non-closing`,
      });
    }
  }

  return { references, errors };
}

module.exports = { parseLinkedIssues, stripHtmlComments };
