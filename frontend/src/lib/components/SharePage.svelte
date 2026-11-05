<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Bubble from '$lib/components/ui/bubble'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import DownloadIcon from '@lucide/svelte/icons/download'
  import FileIcon from '@lucide/svelte/icons/file'
  import FileTextIcon from '@lucide/svelte/icons/file-text'
  import ImageIcon from '@lucide/svelte/icons/image'
  import InboxIcon from '@lucide/svelte/icons/inbox'
  import { api, formatSize, shareFileUrl, type Message } from '$lib/api'
  import { onMount } from 'svelte'

  let { messageId }: { messageId: string } = $props()

  let message = $state<Message | null>(null)
  let failed = $state(false)

  let isImage = $derived((message?.mime ?? '').startsWith('image/'))
  let isText = $derived(
    (message?.mime ?? '').startsWith('text/') ||
      ['application/json', 'application/xml', 'application/yaml', 'application/x-yaml', 'image/svg+xml'].includes(
        message?.mime ?? '',
      ),
  )

  onMount(async () => {
    try {
      message = (await api.share(messageId)).message
    } catch {
      failed = true
    }
  })

  let fileText = $state<string | null>(null)
  $effect(() => {
    if (message?.kind !== 'file' || !isText) return
    let cancelled = false
    fetch(shareFileUrl(message.id))
      .then((resp) => (resp.ok ? resp.text() : Promise.reject()))
      .then((text) => {
        if (!cancelled) fileText = text.length > 200000 ? text.slice(0, 200000) + '\n…' : text
      })
      .catch(() => {
        if (!cancelled) fileText = null
      })
    return () => {
      cancelled = true
    }
  })
</script>

<div class="flex min-h-dvh flex-col">
  <header class="flex items-center gap-2 border-b px-4 py-3">
    <img src="/logo.svg" alt="Fling logo" class="size-6 shrink-0" />
    <span class="text-sm font-semibold tracking-tight">Fling</span>
    <span class="text-sm text-muted-foreground">shared message</span>
  </header>

  <main class="flex flex-1 items-start justify-center p-4">
    {#if failed}
      <div class="flex flex-col items-center gap-2 py-20 text-center">
        <InboxIcon class="size-10 text-muted-foreground" />
        <span class="text-sm font-medium">Message not found</span>
        <span class="text-xs text-muted-foreground">The link may be wrong or the message was deleted.</span>
      </div>
    {:else if !message}
      <div class="flex w-full max-w-md flex-col gap-2 py-10">
        <Skeleton class="h-24 w-full rounded-2xl" />
      </div>
    {:else if message.kind === 'text'}
      <Bubble.Root variant="secondary" class="mt-10 max-w-xl">
        <Bubble.Content class="rounded-2xl px-4 py-3">
          {#if message.sender}
            <div class="mb-0.5 text-xs font-medium opacity-70">{message.sender}</div>
          {/if}
          <div class="text-sm whitespace-pre-wrap break-words">{message.content}</div>
        </Bubble.Content>
      </Bubble.Root>
    {:else}
      <div class="mt-10 w-full max-w-xl">
        {#if isImage}
          <img
            src={shareFileUrl(message.id)}
            alt={message.filename}
            class="mx-auto max-h-96 rounded-xl border object-contain"
          />
        {/if}
        {#if isText && fileText !== null}
          <pre class="mb-3 max-h-96 overflow-auto rounded-xl border bg-muted p-3 text-xs whitespace-pre-wrap break-words">{fileText}</pre>
        {/if}
        <div class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3">
          <span class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
            {#if isImage}
              <ImageIcon class="size-5 text-muted-foreground" />
            {:else if isText}
              <FileTextIcon class="size-5 text-muted-foreground" />
            {:else}
              <FileIcon class="size-5 text-muted-foreground" />
            {/if}
          </span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm font-medium">{message.filename}</span>
            <span class="block text-xs text-muted-foreground">{formatSize(message.size ?? 0)}</span>
          </span>
          <Button size="icon" variant="outline" class="size-8 shrink-0" href={shareFileUrl(message.id, true)} download={message.filename} aria-label="Download">
            <DownloadIcon />
          </Button>
        </div>
      </div>
    {/if}
  </main>
</div>
