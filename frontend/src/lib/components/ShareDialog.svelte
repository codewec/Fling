<script lang="ts">
  import { toast } from 'svelte-sonner'
  import QRCode from 'qrcode'
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import CopyIcon from '@lucide/svelte/icons/copy'
  import { shareUrl, type Message } from '$lib/api'

  let {
    message = null,
    open = $bindable(false),
  }: {
    message: Message | null
    open?: boolean
  } = $props()

  let url = $derived(message ? shareUrl(message.id) : '')
  let qr = $state('')

  $effect(() => {
    if (!open || !url) {
      qr = ''
      return
    }
    let cancelled = false
    QRCode.toDataURL(url, { width: 512, margin: 1 }).then((data) => {
      if (!cancelled) qr = data
    })
    return () => {
      cancelled = true
    }
  })

  function copy() {
    navigator.clipboard.writeText(url)
    toast.success('Link copied')
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-sm">
    <Dialog.Header>
      <Dialog.Title>Share message</Dialog.Title>
      <Dialog.Description>Anyone with this link can view the message.</Dialog.Description>
    </Dialog.Header>
    <div class="flex flex-col items-center gap-4">
      {#if qr}
        <img src={qr} alt="QR code" class="size-52 rounded-lg border bg-white p-2" />
      {:else}
        <div class="size-52 rounded-lg border bg-muted"></div>
      {/if}
      <div class="flex w-full gap-1.5">
        <Input readonly value={url} onclick={(e) => (e.target as HTMLInputElement).select()} aria-label="Share link" />
        <Button size="icon" variant="outline" onclick={copy} aria-label="Copy link">
          <CopyIcon />
        </Button>
      </div>
    </div>
  </Dialog.Content>
</Dialog.Root>
