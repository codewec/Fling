<script lang="ts">
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import * as InputOTP from '$lib/components/ui/input-otp'
  import * as Field from '$lib/components/ui/field'
  import { Spinner } from '$lib/components/ui/spinner'
  import { api, ApiError } from '$lib/api'

  let {
    bucketId,
    open = $bindable(false),
    onUnlocked,
  }: {
    bucketId: string
    open?: boolean
    onUnlocked: () => void
  } = $props()

  let pin = $state('')
  let submitting = $state(false)

  $effect(() => {
    if (pin.length === 4) submit()
  })

  async function submit() {
    if (pin.length !== 4 || submitting) return
    submitting = true
    try {
      await api.unlock(bucketId, pin)
      open = false
      pin = ''
      onUnlocked()
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
      submitting = false
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-xs">
    <Dialog.Header>
      <Dialog.Title>PIN code</Dialog.Title>
      <Dialog.Description>This bucket is protected. Enter the 4-character PIN.</Dialog.Description>
    </Dialog.Header>
    <form
      class="flex flex-col items-center gap-4"
      onsubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Field.Field>
        <Field.FieldLabel for="pin" class="sr-only">PIN</Field.FieldLabel>
        <InputOTP.Root id="pin" bind:value={pin} maxlength={4}>
          {#snippet children({ cells })}
            <InputOTP.Group>
              {#each cells as cell (cell)}
                <InputOTP.Slot {cell} />
              {/each}
            </InputOTP.Group>
          {/snippet}
        </InputOTP.Root>
      </Field.Field>
      <Dialog.Footer>
        <Button type="submit" disabled={pin.length !== 4 || submitting}>
          {#if submitting}
            <Spinner data-icon="inline-start" />
          {/if}
          Unlock
        </Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
