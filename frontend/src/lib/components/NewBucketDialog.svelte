<script lang="ts">
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import { Switch } from '$lib/components/ui/switch'
  import * as Field from '$lib/components/ui/field'
  import { Spinner } from '$lib/components/ui/spinner'
  import { api } from '$lib/api'
  import { push } from '$lib/router'
  import { auth, refreshBuckets } from '$lib/stores'

  let {
    open = $bindable(false),
  }: {
    open?: boolean
  } = $props()

  let name = $state('')
  let usePin = $state(false)
  let pin = $state('')
  let isPrivate = $state(false)
  let submitting = $state(false)

  function reset() {
    name = ''
    usePin = false
    pin = ''
    isPrivate = false
  }

  async function submit() {
    if (!name.trim() || submitting) return
    if (usePin && pin.length !== 4) {
      toast.error('PIN must be exactly 4 characters')
      return
    }
    submitting = true
    try {
      const bucket = await api.createBucket(name.trim(), usePin ? pin : undefined, isPrivate)
      await refreshBuckets()
      open = false
      reset()
      push(`/b/${bucket.id}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create bucket')
    } finally {
      submitting = false
    }
  }
</script>

<Dialog.Root bind:open onOpenChange={(o) => !o && reset()}>
  <Dialog.Content class="sm:max-w-sm">
    <Dialog.Header>
      <Dialog.Title>New bucket</Dialog.Title>
      <Dialog.Description>A bucket is a shared space for messages and files. Everyone on the network can see it.</Dialog.Description>
    </Dialog.Header>
    <form
      class="flex flex-col gap-4"
      onsubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Field.Field>
        <Field.FieldLabel for="bucket-name">Name</Field.FieldLabel>
        <Input id="bucket-name" bind:value={name} placeholder="E.g. Photos from the dacha" maxlength={128} />
      </Field.Field>
      <Field.Field class="flex-row items-center justify-between">
        <Field.FieldLabel for="bucket-pin-switch">PIN (4 characters)</Field.FieldLabel>
        <Switch id="bucket-pin-switch" bind:checked={usePin} />
      </Field.Field>
      {#if usePin}
        <Field.Field>
          <Field.FieldLabel for="bucket-pin">PIN</Field.FieldLabel>
          <Input id="bucket-pin" bind:value={pin} maxlength={4} placeholder="0000" class="tracking-[0.5em]" />
          <Field.FieldDescription>The bucket opens only after the PIN is entered.</Field.FieldDescription>
        </Field.Field>
      {/if}
      {#if $auth?.user}
        <Field.Field class="flex-row items-center justify-between">
          <Field.FieldLabel for="bucket-private-switch">Private bucket</Field.FieldLabel>
          <Switch id="bucket-private-switch" bind:checked={isPrivate} />
        </Field.Field>
        {#if isPrivate}
          <p class="-mt-2 text-xs text-muted-foreground">Visible only to you. Other users will not see it.</p>
        {/if}
      {/if}
      <Dialog.Footer>
        <Button type="submit" disabled={!name.trim() || submitting}>
          {#if submitting}
            <Spinner data-icon="inline-start" />
          {/if}
          Create
        </Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
