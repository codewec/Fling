<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { ModeWatcher, toggleMode, mode } from 'mode-watcher'
  import { Toaster } from '$lib/components/ui/sonner'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import MoonIcon from '@lucide/svelte/icons/moon'
  import SunIcon from '@lucide/svelte/icons/sun'
  import FolderOpenIcon from '@lucide/svelte/icons/folder-open'
  import { routePath } from '$lib/router'
  import { auth, buckets, refreshAuth, refreshBuckets } from '$lib/stores'
  import BucketList from '$lib/components/BucketList.svelte'
  import ChatView from '$lib/components/ChatView.svelte'
  import LoginDialog from '$lib/components/LoginDialog.svelte'
  import SharePage from '$lib/components/SharePage.svelte'

  let bucketId = $derived($routePath.startsWith('/b/') ? $routePath.slice(3) : '')
  let shareId = $derived($routePath.startsWith('/s/') ? $routePath.slice(3) : '')
  let activeBucket = $derived($buckets.find((b) => b.id === bucketId))

  let bucketsInterval = setInterval(refreshBuckets, 15000)
  onDestroy(() => clearInterval(bucketsInterval))

  onMount(() => {
    refreshAuth()
    refreshBuckets()
  })
</script>

<svelte:head>
  <title>Fling</title>
</svelte:head>

<ModeWatcher />
<Toaster position="top-center" richColors />

{#if $auth === null}
  <div class="flex h-dvh flex-col gap-3 p-6">
    <Skeleton class="h-10 w-48" />
    <Skeleton class="h-6 w-72" />
    <Skeleton class="h-40 w-full rounded-xl" />
  </div>
{:else if $auth.auth_required}
  <LoginDialog />
{:else if shareId}
  <SharePage messageId={shareId} />
{:else}
  <div class="flex h-dvh overflow-hidden">
    <aside
      class={bucketId
        ? 'hidden w-80 shrink-0 flex-col border-r bg-card md:flex'
        : 'flex w-full flex-col border-r bg-card md:w-80 md:shrink-0'}
    >
      <div class="min-h-0 flex-1">
        <BucketList {bucketId} />
      </div>
      <div class="flex items-center gap-2 border-t px-4 py-2.5">
        <span class="flex-1 text-xs text-muted-foreground">Local file exchange</span>
        <Button size="icon" variant="ghost" class="size-8" onclick={toggleMode} aria-label="Toggle theme">
          {#if mode.current === 'dark'}
            <SunIcon />
          {:else}
            <MoonIcon />
          {/if}
        </Button>
      </div>
    </aside>

    <main class={bucketId ? 'relative min-w-0 flex-1' : 'hidden min-w-0 flex-1 md:block'}>
      {#if bucketId}
        <ChatView {bucketId} bucketName={activeBucket?.name ?? ''} />
      {:else}
        <div class="flex h-full flex-col items-center justify-center gap-3 text-center">
          <FolderOpenIcon class="size-12 text-muted-foreground" />
          <div class="flex flex-col gap-1">
            <span class="text-lg font-semibold">Select a bucket</span>
            <span class="text-sm text-muted-foreground">
              Or create a new one and share text and files across your LAN
            </span>
          </div>
        </div>
      {/if}
    </main>
  </div>
{/if}
