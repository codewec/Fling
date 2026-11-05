<script lang="ts">
  import { toast } from 'svelte-sonner'
  import { formatSize, formatTime, fileUrl, type Message } from '$lib/api'
  import { cn } from '$lib/utils.js'
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import * as MessageUI from '$lib/components/ui/message'
  import * as Bubble from '$lib/components/ui/bubble'
  import FileIcon from '@lucide/svelte/icons/file'
  import FileTextIcon from '@lucide/svelte/icons/file-text'
  import ImageIcon from '@lucide/svelte/icons/image'
  import DownloadIcon from '@lucide/svelte/icons/download'
  import EyeIcon from '@lucide/svelte/icons/eye'
  import PinIcon from '@lucide/svelte/icons/pin'
  import PinOffIcon from '@lucide/svelte/icons/pin-off'
  import TrashIcon from '@lucide/svelte/icons/trash-2'
  import CopyIcon from '@lucide/svelte/icons/copy'

  let {
    message,
    highlighted = false,
    onTogglePin,
    onDelete,
  }: {
    message: Message
    highlighted?: boolean
    onTogglePin: (message: Message) => void
    onDelete: (message: Message) => void
  } = $props()

  // HEIC/HEIF can hardly be rendered by browsers — treat them as regular files
  const NON_PREVIEWABLE_IMAGES = ['image/heic', 'image/heif', 'image/heic-sequence', 'image/heif-sequence']

  // Structured text formats whose MIME type is not text/* but preview fine
  const TEXT_LIKE_MIMES = [
    'application/json',
    'application/ld+json',
    'application/xml',
    'application/xhtml+xml',
    'application/yaml',
    'application/x-yaml',
    'text/yaml',
    'application/toml',
    'application/javascript',
    'application/x-javascript',
    'application/x-sh',
    'application/sql',
    'image/svg+xml',
  ]

  function isTextLike(mime: string): boolean {
    return mime.startsWith('text/') || TEXT_LIKE_MIMES.includes(mime) || mime.endsWith('+json') || mime.endsWith('+xml')
  }

  let mime = $derived((message.mime ?? '').toLowerCase())
  let isImage = $derived(mime.startsWith('image/') && !NON_PREVIEWABLE_IMAGES.includes(mime))
  let isText = $derived(isTextLike(mime))
  let isPdf = $derived(mime === 'application/pdf')
  let canPreview = $derived(isText || isImage || isPdf)

  let previewOpen = $state(false)
  let previewKind = $state<'text' | 'image' | 'pdf' | null>(null)
  let textPreview = $state<string | null>(null)

  async function openPreview() {
    if (isText && textPreview === null) {
      try {
        const resp = await fetch(fileUrl(message.id, true))
        if (!resp.ok) throw new Error()
        const text = await resp.text()
        textPreview = text.length > 200000 ? text.slice(0, 200000) + '\n…' : text
      } catch {
        textPreview = null
        return
      }
    }
    previewKind = isText ? 'text' : isPdf ? 'pdf' : 'image'
    previewOpen = true
  }

  function copyText() {
    if (!message.content) return
    navigator.clipboard.writeText(message.content)
    toast.success('Copied')
  }
</script>

<MessageUI.Root>
  <MessageUI.Content>
    {#if message.kind === 'text'}
      <Bubble.Root variant="secondary" class={highlighted ? 'bubble-flash' : ''}>
        <Bubble.Content class="rounded-2xl rounded-bl-md px-3.5 py-2">
          <div class="text-sm whitespace-pre-wrap break-words">{message.content}</div>
        </Bubble.Content>
      </Bubble.Root>
    {:else if message.kind === 'file'}
      {#if isImage}
        <button
          type="button"
          class={cn('block w-fit max-w-[80%] cursor-zoom-in overflow-hidden rounded-2xl rounded-bl-md bg-muted p-1.5 text-left', highlighted && 'animate-[bubble-flash_1.5s_ease-out]')}
          onclick={openPreview}
          aria-label="Open image"
        >
          <img
            src={fileUrl(message.id, true)}
            alt={message.filename}
            class="max-h-72 max-w-full rounded-xl object-contain"
          />
          <div class="flex items-center gap-2 px-1.5 py-1">
            <ImageIcon class="size-4 shrink-0 text-muted-foreground" />
            <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">{message.filename}</span>
            <a
              class="flex size-7 shrink-0 items-center justify-center rounded-md hover:bg-accent hover:text-accent-foreground"
              href={fileUrl(message.id)}
              download={message.filename}
              aria-label="Download"
              onclick={(e) => e.stopPropagation()}
            >
              <DownloadIcon class="size-4" />
            </a>
          </div>
        </button>
      {:else}
        <Bubble.Root variant="muted" class={cn('max-w-[80%]', highlighted && 'bubble-flash')}>
          <Bubble.Content class="rounded-2xl rounded-bl-md px-3.5 py-3">
            <div class="flex w-72 max-w-full items-center gap-3">
              <span class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-background">
                {#if isText || isPdf}
                  <FileTextIcon class="size-5 text-muted-foreground" />
                {:else}
                  <FileIcon class="size-5 text-muted-foreground" />
                {/if}
              </span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-sm font-medium">{message.filename}</span>
                <span class="block text-xs text-muted-foreground">{formatSize(message.size ?? 0)}</span>
              </span>
              <Button
                size="icon"
                variant="ghost"
                class="size-8 shrink-0"
                href={fileUrl(message.id)}
                download={message.filename}
                aria-label="Download"
              >
                <DownloadIcon />
              </Button>
            </div>
            {#if canPreview}
              <Button size="sm" variant="outline" class="mt-2" onclick={openPreview}>
                <EyeIcon data-icon="inline-start" />
                Preview
              </Button>
            {/if}
          </Bubble.Content>
        </Bubble.Root>
      {/if}
    {/if}
    <div class="flex items-center gap-0.5 px-1 text-muted-foreground">
      {#if message.kind === 'text'}
        <Button size="icon" variant="ghost" class="size-6" onclick={copyText} aria-label="Copy">
          <CopyIcon class="size-3.5" />
        </Button>
      {/if}
      <Button
        size="icon"
        variant="ghost"
        class="size-6"
        onclick={() => onTogglePin(message)}
        aria-label={message.pinned ? 'Unpin' : 'Pin'}
      >
        {#if message.pinned}
          <PinOffIcon class="size-3.5" />
        {:else}
          <PinIcon class="size-3.5" />
        {/if}
      </Button>
      <Button
        size="icon"
        variant="ghost"
        class="size-6 hover:text-destructive"
        onclick={() => onDelete(message)}
        aria-label="Delete"
      >
        <TrashIcon class="size-3.5" />
      </Button>
      <span class="ml-1 text-[11px]">
        {message.sender ? `${message.sender} · ` : ''}{formatTime(message.created_at)}
      </span>
    </div>
  </MessageUI.Content>
</MessageUI.Root>

<Dialog.Root bind:open={previewOpen}>
  <Dialog.Content class="max-h-[85vh] overflow-auto sm:max-w-3xl">
    <Dialog.Header>
      <Dialog.Title class="truncate">{message.filename ?? 'Preview'}</Dialog.Title>
      <Dialog.Description class="sr-only">
        Previewing file {message.filename}
      </Dialog.Description>
    </Dialog.Header>
    {#if previewKind === 'text'}
      <pre class="max-h-[65vh] overflow-auto rounded-md bg-muted p-3 text-xs whitespace-pre-wrap break-words">{textPreview ?? 'Loading…'}</pre>
    {:else if previewKind === 'image'}
      <img src={fileUrl(message.id, true)} alt={message.filename ?? ''} class="mx-auto max-h-[65vh] max-w-full rounded-lg object-contain" />
    {:else if previewKind === 'pdf'}
      <iframe
        src={fileUrl(message.id, true)}
        title={message.filename ?? 'PDF'}
        class="h-[65vh] w-full rounded-lg border bg-background"
      ></iframe>
    {/if}
    <Dialog.Footer>
      <Button href={fileUrl(message.id)} download={message.filename} variant="outline">
        <DownloadIcon data-icon="inline-start" />
        Download
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
