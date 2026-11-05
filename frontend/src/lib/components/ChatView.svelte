<script lang="ts">
  import { onDestroy } from 'svelte'
  import { toast } from 'svelte-sonner'
  import SendIcon from '@lucide/svelte/icons/send-horizontal'
  import PaperclipIcon from '@lucide/svelte/icons/paperclip'
  import UserIcon from '@lucide/svelte/icons/user-round'
  import { Button } from '$lib/components/ui/button'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu'
  import * as InputGroup from '$lib/components/ui/input-group'
  import * as MessageScroller from '$lib/components/ui/message-scroller'
  import { Progress } from '$lib/components/ui/progress'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import { Spinner } from '$lib/components/ui/spinner'
  import { Input } from '$lib/components/ui/input'
  import { api, ApiError, uploadFile, type Message } from '$lib/api'
  import { getSender, setSender, refreshAuth } from '$lib/stores'
  import MessageBubble from './MessageBubble.svelte'
  import PinnedBar from './PinnedBar.svelte'
  import * as InputOTP from '$lib/components/ui/input-otp'
  import LockIcon from '@lucide/svelte/icons/lock-keyhole'

  let { bucketId, bucketName }: { bucketId: string; bucketName: string } = $props()

  let messages = $state<Message[]>([])
  let pinnedMessages = $state<Message[]>([])
  let loading = $state(true)
  let needsPin = $state(false)
  let pin = $state('')
  let pinSubmitting = $state(false)
  let deleteTarget = $state<Message | null>(null)
  let deleting = $state(false)
  let text = $state('')
  let sender = $state(getSender())
  let senderDraft = $state(getSender())
  let senderOpen = $state(false)
  let dragOver = $state(false)
  let upload = $state<{ name: string; fraction: number } | null>(null)
  let sending = $state(false)
  let highlightedId = $state('')
  let highlightTimer: ReturnType<typeof setTimeout> | undefined

  function jumpToMessage(message: Message) {
    highlightedId = message.id
    clearTimeout(highlightTimer)
    highlightTimer = setTimeout(() => (highlightedId = ''), 1600)
  }

  let fileInput = $state<HTMLInputElement | null>(null)
  let composerForm = $state<HTMLFormElement | null>(null)

  function handleLoadError(err: unknown) {
    if (err instanceof ApiError && err.status === 403) {
      needsPin = true
    } else if (err instanceof ApiError && err.status === 401) {
      refreshAuth()
    }
  }

  async function submitPin() {
    if (pin.length !== 4 || pinSubmitting) return
    pinSubmitting = true
    try {
      await api.unlock(bucketId, pin)
      pin = ''
      await loadAll()
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        const retry = typeof err.data.retry_after === 'number' ? err.data.retry_after : 0
        toast.error(`Too many attempts. Try again in ${Math.ceil(retry / 60) > 1 ? `${Math.ceil(retry / 60)} min` : `${retry} sec`}.`)
      } else if (err instanceof ApiError && err.status === 401) {
        const left = err.data.attempts_remaining
        toast.error(left !== undefined ? `Wrong PIN. Attempts remaining: ${left}.` : 'Wrong PIN.')
        pin = ''
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to unlock bucket')
      }
    } finally {
      pinSubmitting = false
    }
  }

  async function loadAll() {
    loading = true
    needsPin = false
    messages = []
    pinnedMessages = []
    pin = ''
    try {
      const data = await api.messages(bucketId)
      messages = data.messages
      pinnedMessages = data.pinned_messages
    } catch (err) {
      handleLoadError(err)
    } finally {
      loading = false
    }
  }

  async function poll() {
    if (document.hidden) return
    const after = messages.length > 0 ? messages[messages.length - 1].id : ''
    try {
      const data = await api.messages(bucketId, after)
      pinnedMessages = data.pinned_messages
      if (data.messages.length > 0) {
        messages = [...messages, ...data.messages]
      }
    } catch (err) {
      handleLoadError(err)
    }
  }

  async function send() {
    const content = text.trim()
    if (!content || sending) return
    sending = true
    try {
      const msg = await api.sendMessage(bucketId, content, sender)
      text = ''
      messages = [...messages, msg]
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) {
        needsPin = true
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to send')
      }
    } finally {
      sending = false
    }
  }

  async function uploadFiles(files: Iterable<File>) {
    for (const file of files) {
      upload = { name: file.name, fraction: 0 }
      try {
        const msg = await uploadFile(bucketId, file, sender, (f) => {
          if (upload) upload = { ...upload, fraction: f }
        })
        messages = [...messages, msg]
      } catch (err) {
        if (err instanceof ApiError && err.status === 403) {
          needsPin = true
          return
        }
        toast.error(err instanceof Error ? `${file.name}: ${err.message}` : `Failed to upload ${file.name}`)
      } finally {
        upload = null
      }
    }
  }

  async function togglePin(message: Message) {
    try {
      await api.pinMessage(message.id, !message.pinned)
      message.pinned = !message.pinned
      pinnedMessages = message.pinned
        ? [...pinnedMessages.filter((m) => m.id !== message.id), message]
        : pinnedMessages.filter((m) => m.id !== message.id)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to pin')
    }
  }

  async function confirmDelete() {
    if (!deleteTarget || deleting) return
    deleting = true
    try {
      await api.deleteMessage(deleteTarget.id)
      messages = messages.filter((m) => m.id !== deleteTarget!.id)
      pinnedMessages = pinnedMessages.filter((m) => m.id !== deleteTarget!.id)
      toast.success('Message deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete')
    } finally {
      deleting = false
      deleteTarget = null
    }
  }

  function saveSender() {
    sender = senderDraft.trim()
    setSender(sender)
    senderOpen = false
  }

  function onComposerKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      composerForm?.requestSubmit()
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    dragOver = false
    if (e.dataTransfer?.files?.length) uploadFiles(e.dataTransfer.files)
  }

  let interval = setInterval(poll, 3000)
  onDestroy(() => {
    clearInterval(interval)
    clearTimeout(highlightTimer)
  })

  $effect(() => {
    // reset per-bucket state when navigating
    bucketId
    text = ''
    loadAll()
  })
</script>

<div
  class="relative flex h-full min-h-0 flex-col"
  role="region"
  aria-label="Bucket messages"
  ondragover={(e) => {
    e.preventDefault()
    dragOver = true
  }}
  ondragleave={() => (dragOver = false)}
  ondrop={onDrop}
>
  <div class="flex items-center gap-2 border-b px-4 py-2.5 md:hidden">
    <a href="/" class="text-sm text-muted-foreground">← Buckets</a>
    <span class="truncate text-sm font-semibold">{bucketName || '…'}</span>
  </div>

  <MessageScroller.Provider autoScroll>
    {#if pinnedMessages.length > 0}
      <PinnedBar messages={pinnedMessages} onJump={jumpToMessage} onUnpin={togglePin} />
    {/if}

    <MessageScroller.Root class="min-h-0 flex-1">
      <MessageScroller.Viewport>
        <MessageScroller.Content class="px-4 py-6">
          {#if loading}
            <div class="flex flex-col gap-3">
              <Skeleton class="h-10 w-2/3 rounded-2xl" />
              <Skeleton class="h-16 w-1/2 rounded-2xl" />
              <Skeleton class="h-10 w-3/4 rounded-2xl" />
            </div>
          {:else if needsPin}
            <div class="flex min-h-full flex-col items-center justify-center gap-5 py-16 text-center">
              <span class="flex size-14 items-center justify-center rounded-full bg-muted">
                <LockIcon class="size-6 text-muted-foreground" />
              </span>
              <div class="flex flex-col gap-1">
                <span class="text-base font-semibold">This bucket is protected</span>
                <span class="text-sm text-muted-foreground">Enter the 4-character PIN to open it</span>
              </div>
              <InputOTP.Root
                bind:value={pin}
                maxlength={4}
                disabled={pinSubmitting}
                aria-label="PIN"
                onComplete={submitPin}
              >
                {#snippet children({ cells })}
                  <InputOTP.Group>
                    {#each cells as cell (cell)}
                      <InputOTP.Slot {cell} class="size-14 text-2xl" />
                    {/each}
                  </InputOTP.Group>
                {/snippet}
              </InputOTP.Root>
            </div>
          {:else if messages.length === 0}
            <div class="py-16 text-center text-sm text-muted-foreground">
              Nothing here yet. Send a message or drop a file.
            </div>
          {:else}
            {#each messages as message (message.id)}
              <MessageScroller.Item messageId={message.id}>
                <MessageBubble {message} highlighted={message.id === highlightedId} onTogglePin={togglePin} onDelete={(m) => (deleteTarget = m)} />
              </MessageScroller.Item>
            {/each}
          {/if}
        </MessageScroller.Content>
      </MessageScroller.Viewport>
      <MessageScroller.Button />
    </MessageScroller.Root>
  </MessageScroller.Provider>

  {#if upload}
    <div class="border-t px-4 py-2">
      <div class="flex items-center gap-3">
        <Spinner class="size-4 shrink-0" />
        <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">{upload.name}</span>
        <span class="text-xs text-muted-foreground">{Math.round(upload.fraction * 100)}%</span>
      </div>
      <Progress value={upload.fraction * 100} class="mt-1.5" />
    </div>
  {/if}

  <div class="p-3">
    <form
      bind:this={composerForm}
      class="w-full"
      onsubmit={(e) => {
        e.preventDefault()
        send()
      }}
    >
      <InputGroup.Root>
        <InputGroup.Textarea
          bind:value={text}
          onkeydown={onComposerKeydown}
          placeholder="Message…"
          aria-label="Message"
          class="min-h-12"
          rows={1}
        />
        <InputGroup.Addon align="block-end" class="pt-1">
          <input
            bind:this={fileInput}
            type="file"
            multiple
            class="hidden"
            onchange={(e) => {
              const files = (e.target as HTMLInputElement).files
              if (files?.length) uploadFiles(files)
              ;(e.target as HTMLInputElement).value = ''
            }}
          />
          <InputGroup.Button
            type="button"
            variant="outline"
            size="icon-sm"
            aria-label="Attach file"
            onclick={() => fileInput?.click()}
          >
            <PaperclipIcon />
          </InputGroup.Button>
          <DropdownMenu.Root bind:open={senderOpen}>
            <DropdownMenu.Trigger>
              {#snippet child({ props })}
                <InputGroup.Button {...props} type="button" variant="outline" size="sm">
                  <UserIcon data-icon="inline-start" />
                  <span class="max-w-28 truncate">{sender || 'anonymous'}</span>
                </InputGroup.Button>
              {/snippet}
            </DropdownMenu.Trigger>
            <DropdownMenu.Content align="start" side="top" class="w-56">
              <div class="flex flex-col gap-1.5 p-2">
                <span class="text-xs font-medium text-muted-foreground">Sender name</span>
                <div class="flex gap-1.5">
                  <Input
                    bind:value={senderDraft}
                    placeholder="anonymous"
                    maxlength={32}
                    onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), saveSender())}
                  />
                  <Button type="button" size="sm" onclick={saveSender}>OK</Button>
                </div>
              </div>
            </DropdownMenu.Content>
          </DropdownMenu.Root>
          <InputGroup.Button
            type="submit"
            variant="default"
            size="icon-sm"
            disabled={!text.trim() || sending}
            class="ml-auto"
            aria-label="Send"
          >
            {#if sending}
              <Spinner />
            {:else}
              <SendIcon />
            {/if}
          </InputGroup.Button>
        </InputGroup.Addon>
      </InputGroup.Root>
    </form>
  </div>

  {#if dragOver}
    <div class="pointer-events-none absolute inset-0 z-10 flex items-center justify-center bg-background/80">
      <span class="rounded-xl border-2 border-dashed border-primary px-8 py-6 text-sm font-medium">
        Drop files to upload
      </span>
    </div>
  {/if}
</div>

<AlertDialog.Root open={deleteTarget !== null} onOpenChange={(o) => !o && (deleteTarget = null)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Delete message?</AlertDialog.Title>
      <AlertDialog.Description>
        {#if deleteTarget?.kind === 'file'}
          The file "{deleteTarget?.filename}" will be permanently deleted.
        {:else}
          This action cannot be undone.
        {/if}
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
