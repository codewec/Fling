<script lang="ts">
  import { toast } from 'svelte-sonner'
  import LockIcon from '@lucide/svelte/icons/lock'
  import PinIcon from '@lucide/svelte/icons/pin'
  import PinOffIcon from '@lucide/svelte/icons/pin-off'
  import PlusIcon from '@lucide/svelte/icons/plus'
  import SearchIcon from '@lucide/svelte/icons/search'
  import TrashIcon from '@lucide/svelte/icons/trash-2'
  import LinkIcon from '@lucide/svelte/icons/link'
  import LockOpenIcon from '@lucide/svelte/icons/lock-open'
  import UserRoundIcon from '@lucide/svelte/icons/user-round'
  import { Button } from '$lib/components/ui/button'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import * as Avatar from '$lib/components/ui/avatar'
  import * as ContextMenu from '$lib/components/ui/context-menu'
  import { Input } from '$lib/components/ui/input'
  import { cn } from '$lib/utils.js'
  import type { Bucket } from '$lib/api'
  import { api } from '$lib/api'
  import { push } from '$lib/router'
  import { buckets, auth, refreshBuckets } from '$lib/stores'
  import NewBucketDialog from './NewBucketDialog.svelte'

  let { bucketId = '' }: { bucketId?: string } = $props()

  let search = $state('')
  let newOpen = $state(false)
  let deleteTarget = $state<Bucket | null>(null)
  let deleting = $state(false)

  let filtered = $derived(
    search.trim()
      ? $buckets.filter((b) => b.name.toLowerCase().includes(search.trim().toLowerCase()))
      : $buckets,
  )

  async function confirmDelete() {
    if (!deleteTarget || deleting) return
    deleting = true
    try {
      await api.deleteBucket(deleteTarget.id)
      const wasActive = deleteTarget.id === bucketId
      await refreshBuckets()
      if (wasActive) push('/')
      toast.success('Bucket deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete bucket')
    } finally {
      deleting = false
      deleteTarget = null
    }
  }

  function copyLink(bucket: Bucket) {
    navigator.clipboard.writeText(`${window.location.origin}/b/${bucket.id}`)
    toast.success('Link copied')
  }

  async function lockAgain(bucket: Bucket) {
    await api.lock(bucket.id)
    if (bucket.id === bucketId) push('/')
    else toast.success('Bucket locked')
  }

  async function togglePin(bucket: Bucket) {
    try {
      await api.pinBucket(bucket.id, !bucket.pinned)
      await refreshBuckets()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to pin')
    }
  }
</script>

<div class="flex h-full min-h-0 flex-col">
  <div class="flex items-center gap-2 px-4 pt-4 pb-3">
    <img src="/logo.svg" alt="Fling logo" class="size-7 shrink-0" />
    <span class="flex-1 text-lg font-semibold tracking-tight">Fling</span>
    {#if $auth?.user}
      <span
        class="flex max-w-32 items-center gap-1 rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground"
        title="Signed in as {$auth.user}"
      >
        <UserRoundIcon class="size-3.5 shrink-0" />
        <span class="truncate">{$auth.user}</span>
      </span>
    {/if}
    <Button size="sm" onclick={() => (newOpen = true)}>
      <PlusIcon data-icon="inline-start" />
      New bucket
    </Button>
  </div>

  <div class="px-4 pb-3">
    <div class="relative">
      <SearchIcon class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input bind:value={search} placeholder="Search" class="pl-9" />
    </div>
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
    {#if filtered.length === 0}
      <div class="flex flex-col items-center gap-1 px-4 py-10 text-center">
        <span class="text-sm font-medium text-muted-foreground">
          {$buckets.length === 0 ? 'No buckets yet' : 'No matches found'}
        </span>
        {#if $buckets.length === 0}
          <span class="text-xs text-muted-foreground">Create the first bucket to start exchanging</span>
        {/if}
      </div>
    {/if}
    {#each filtered as bucket (bucket.id)}
      <ContextMenu.Root>
        <ContextMenu.Trigger>
          {#snippet child({ props })}
            <a
              {...props}
              href="/b/{bucket.id}"
              class={cn(
                'mb-1 flex items-center gap-2.5 rounded-lg py-2 pr-3 pl-2 transition-colors last:mb-0 hover:bg-accent/60',
                bucket.id === bucketId && 'bg-accent hover:bg-accent',
              )}
            >
              <Avatar.Root class="size-9 shrink-0">
                <Avatar.Fallback
                  class={cn(
                    'bg-muted text-sm font-medium text-muted-foreground',
                    bucket.pinned && 'bg-primary/10 text-primary',
                  )}
                >
                  {#if bucket.pinned}
                    <PinIcon class="size-4" />
                  {:else}
                    {bucket.name.trim().charAt(0).toUpperCase() || '?'}
                  {/if}
                </Avatar.Fallback>
              </Avatar.Root>
              <span class="flex min-w-0 flex-1 flex-col">
                <span class="truncate text-sm font-medium">{bucket.name}</span>
                <span class="text-xs text-muted-foreground">
                  {new Date(bucket.created_at * 1000).toLocaleDateString(undefined, {
                    day: 'numeric',
                    month: 'short',
                    year: 'numeric',
                  })}
                </span>
              </span>
              {#if bucket.private}
                <UserRoundIcon class="size-3.5 shrink-0 text-muted-foreground" aria-label="Private bucket" />
              {/if}
              {#if bucket.has_pin}
                <LockIcon class="size-3.5 shrink-0 text-muted-foreground" />
              {/if}
            </a>
          {/snippet}
        </ContextMenu.Trigger>
        <ContextMenu.Content>
          <ContextMenu.Group>
            <ContextMenu.Item onclick={() => togglePin(bucket)}>
              {#if bucket.pinned}
                <PinOffIcon data-icon="inline-start" />
                Unpin
              {:else}
                <PinIcon data-icon="inline-start" />
                Pin
              {/if}
            </ContextMenu.Item>
            <ContextMenu.Item onclick={() => copyLink(bucket)}>
              <LinkIcon data-icon="inline-start" />
              Copy link
            </ContextMenu.Item>
            {#if bucket.has_pin}
              <ContextMenu.Item onclick={() => lockAgain(bucket)}>
                <LockOpenIcon data-icon="inline-start" />
                Lock
              </ContextMenu.Item>
            {/if}
            <ContextMenu.Separator />
            <ContextMenu.Item variant="destructive" onclick={() => (deleteTarget = bucket)}>
              <TrashIcon data-icon="inline-start" />
              Delete
            </ContextMenu.Item>
          </ContextMenu.Group>
        </ContextMenu.Content>
      </ContextMenu.Root>
    {/each}
  </div>
</div>

<NewBucketDialog bind:open={newOpen} />

<AlertDialog.Root open={deleteTarget !== null} onOpenChange={(o) => !o && (deleteTarget = null)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Delete bucket?</AlertDialog.Title>
      <AlertDialog.Description>
        "{deleteTarget?.name}" and all of its messages and files will be permanently deleted.
      </AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={deleting}>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action variant="destructive" onclick={confirmDelete} disabled={deleting}>
        Delete
      </AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
