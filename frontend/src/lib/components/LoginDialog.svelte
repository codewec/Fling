<script lang="ts">
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import * as Field from '$lib/components/ui/field'
  import { Spinner } from '$lib/components/ui/spinner'
  import { api } from '$lib/api'
  import { refreshAuth } from '$lib/stores'

  let password = $state('')
  let submitting = $state(false)

  async function submit() {
    if (!password || submitting) return
    submitting = true
    try {
      await api.login(password)
      password = ''
      await refreshAuth()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Login failed')
    } finally {
      submitting = false
    }
  }
</script>

<Dialog.Root open={true}>
  <Dialog.Content class="sm:max-w-sm" interactOutsideBehavior="ignore">
    <Dialog.Header>
      <Dialog.Title>Password required</Dialog.Title>
      <Dialog.Description>This server is protected by a shared password. Enter it to continue.</Dialog.Description>
    </Dialog.Header>
    <form
      class="flex flex-col gap-4"
      onsubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Field.Field>
        <Field.FieldLabel for="password">Password</Field.FieldLabel>
        <Input id="password" type="password" bind:value={password} autocomplete="current-password" />
      </Field.Field>
      <Dialog.Footer>
        <Button type="submit" disabled={!password || submitting}>
          {#if submitting}
            <Spinner data-icon="inline-start" />
          {/if}
          Sign in
        </Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
