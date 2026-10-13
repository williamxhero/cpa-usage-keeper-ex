import { describe, expect, it } from 'vitest'
import { compareModelNames } from '../modelSort'

// Preserve shared ordering invariants formerly exercised through the retired price editor adapter.
describe('shared model ordering', () => {
  it.each([
    { name: 'case-insensitive family priority with alphabetical remaining families', ordered: ['GPT-5.6', 'Claude-opus-4.6', 'Gemini-2.5-pro', 'Alpha', 'deepseek-v3', 'gptish-1', 'Qwen3'] },
    { name: 'natural versions, base models and natural ascending suffixes', ordered: ['gpt-5.10', 'gpt-5.9', 'gpt-5.6', 'gpt-5.6-preview-2', 'gpt-5.6-preview-10', 'gpt-5.6-sol', 'gpt-5.6-terra'] },
    { name: 'Claude subseries and both dotted and hyphenated version formats', ordered: ['claude-opus-4.6', 'claude-opus-4.5', 'claude-sonnet-4-6', 'claude-sonnet-4-5', 'claude-3-7-sonnet-20250219', 'claude-3-5-sonnet-20241022'] },
    { name: 'dates remain suffixes after their base version', ordered: ['claude-sonnet-4-6', 'claude-sonnet-4-6-20260901', 'claude-sonnet-4-5', 'claude-sonnet-4-5-20250929'] },
    { name: 'unknown families and names without versions', ordered: ['deepseek-v3.10', 'deepseek-v3.9', 'model-alpha', 'model-beta', 'qwen3.5', 'qwen3'] },
    { name: 'provider-prefixed models mixed with bare names', ordered: ['openai/gpt-5.10', 'gpt-5.9', 'openai/gpt-4o', 'anthropic/claude-opus-4.6', 'google/gemini-2.5-pro', 'deepseek-v3'] },
    { name: 'nested and numbered prefixes before version and Claude parsing', ordered: ['team1/openai/gpt-5.10', 'team2/gpt-5.6', 'team/anthropic/claude-opus-4.6', 'claude-opus-4.5', 'team/claude-sonnet-4-6', 'team/claude-3-7-sonnet-20250219'] },
    { name: 'full-name tie-breaks between prefixes for the same model', ordered: ['zeta/openai/gpt-5.6', 'gpt-5.6', 'alpha/gpt-5.6'] },
    { name: 'exact-name tie-break for naturally equivalent names', ordered: ['gpt-2', 'gpt-02', 'GPT-2'] },
  ])('sorts $name independently of input order without changing model identities', ({ ordered }) => {
    for (const models of [[...ordered].reverse(), [...ordered.slice(2), ...ordered.slice(0, 2)]]) {
      const original = [...models]
      expect([...models].sort(compareModelNames)).toEqual(ordered)
      expect(models).toEqual(original)
    }
  })
})
