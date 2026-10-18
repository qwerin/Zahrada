import { useQuery } from '@tanstack/react-query'
import { CameraIcon } from 'lucide-react'
import { useRef } from 'react'
import { toast } from 'sonner'
import { photoUrl, quoteQueries, useUploadJobPhoto } from '@/api/queries/garden'
import { Button } from '@/components/ui/button'
import { formatDateTime } from '@/lib/date'

/** Fotky zakázky: mřížka náhledů + tlačítko Vyfotit/Přidat (na mobilu otevře fotoaparát). */
export function JobPhotos({ slug, jobId, asContractor, canUpload = true }: { slug: string; jobId: number; asContractor: boolean; canUpload?: boolean }) {
  const q = useQuery(asContractor ? quoteQueries.myJobPhotos(slug, jobId) : quoteQueries.jobPhotos(slug, jobId))
  const upload = useUploadJobPhoto(slug, jobId, asContractor)
  const input = useRef<HTMLInputElement>(null)
  const onFiles = async (files: FileList | null) => {
    if (!files?.length) return
    for (const f of Array.from(files)) {
      try {
        await upload.mutateAsync(f)
      } catch (e) {
        toast.error((e as Error).message)
        return
      }
    }
    toast.success(files.length === 1 ? 'Fotka nahrána' : `Nahráno ${files.length} fotek`)
    if (input.current) input.current.value = ''
  }
  const photos = q.data ?? []
  return (
    <div className="grid gap-2">
      {canUpload && (
        <div>
          <input ref={input} type="file" accept="image/*" capture="environment" multiple hidden onChange={(e) => void onFiles(e.target.files)} />
          <Button size="sm" variant="outline" disabled={upload.isPending} onClick={() => input.current?.click()}>
            <CameraIcon data-icon="inline-start" /> {upload.isPending ? 'Nahrávám…' : 'Vyfotit / přidat fotky'}
          </Button>
        </div>
      )}
      {photos.length === 0 ? (
        <p className="text-sm text-muted-foreground">Zatím žádné fotky.</p>
      ) : (
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
          {photos.map((p) => (
            <a key={p.id} href={photoUrl(slug, p.id, asContractor)} target="_blank" rel="noreferrer" className="group relative block overflow-hidden rounded-md border">
              <img src={photoUrl(slug, p.id, asContractor)} alt={p.filename} loading="lazy" className="aspect-square w-full object-cover" />
              <span className="absolute inset-x-0 bottom-0 truncate bg-black/50 px-1 py-0.5 text-[10px] text-white">
                {[p.uploaded_by, formatDateTime(p.created_at)].filter(Boolean).join(' · ')}
              </span>
            </a>
          ))}
        </div>
      )}
    </div>
  )
}
