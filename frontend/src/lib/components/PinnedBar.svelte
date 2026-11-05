<script lang="ts">
  import { useMessageScroller } from '@shadcn-svelte/primitives/message-scroller'
  import PinIcon from '@lucide/svelte/icons/pin'
  import PinOffIcon from '@lucide/svelte/icons/pin-off'
  import type { Message } from '$lib/api'

  let {
    messages,
    onJump,
    onUnpin,
  }: {
    messages: Message[]
    onJump: (message: Message) => void
    onUnpin: (message: Message) => void
  } = $props()

  // useMessageScroller must be called during component init,
  // inside the MessageScroller.Provider context
  const { scrollToMessage } = useMessageScroller()

  function jump(message: Message) {
    scrollToMessage(message.id, { behavior: 'smooth' })
    onJump(message)
  }
</script>

<div class="flex flex-col gap-1 border-b bg-muted/40 px-4 py-2">
  {#each messages as message (message.id)}
    <div class="flex items-center gap-2 text-sm">
      <button
        type="button"
        class="flex min-w-0 flex-1 items-center gap-2 rounded-md text-left hover:bg-accent/60"
        onclick={() => jump(message)}
        aria-label="Jump to message"
      >
        <PinIcon class="size-3.5 shrink-0 text-muted-foreground" />
        {#if message.kind === 'text'}
          <span class="min-w-0 flex-1 truncate">{message.content}</span>
        {:else}
          <span class="min-w-0 flex-1 truncate font-medium">{message.filename}</span>
        {/if}
      </button>
      <button
        class="shrink-0 rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
        onclick={() => onUnpin(message)}
        aria-label="Unpin"
      >
        <PinOffIcon class="size-3.5" />
      </button>
    </div>
  {/each}
</div>
