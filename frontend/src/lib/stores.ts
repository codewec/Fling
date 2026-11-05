import { writable } from 'svelte/store'
import { api, type AuthState, type Bucket } from './api'

export const auth = writable<AuthState | null>(null)

export async function refreshAuth() {
  try {
    auth.set(await api.me())
  } catch {
    auth.set(null)
  }
}

export const buckets = writable<Bucket[]>([])

export async function refreshBuckets() {
  try {
    buckets.set((await api.buckets()).buckets)
  } catch {
    /* auth gate handles 401s */
  }
}

const SENDER_KEY = 'fling_sender'

export function getSender(): string {
  return localStorage.getItem(SENDER_KEY) ?? ''
}

export function setSender(value: string) {
  localStorage.setItem(SENDER_KEY, value)
}
