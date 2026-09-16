import { createWorker, PSM } from 'tesseract.js'
import { readFileSync } from 'node:fs'
import { createInterface } from 'node:readline'

const input = createInterface({ input: process.stdin, crlfDelay: Infinity })

for await (const line of input) {
  if (!line.trim()) continue
  try {
    const request = JSON.parse(line)
    const image = Buffer.from(request.image_base64, 'base64')
    const languages = Array.isArray(request.languages) && request.languages.length ? request.languages : ['eng']
    const texts = []
    let confidence = 100
    for (const language of languages) {
      const worker = await createWorker(language, 1, {
        langPath: request.lang_path,
        gzip: true,
        cacheMethod: 'none',
      })
      try {
        await worker.setParameters({ tessedit_pageseg_mode: PSM.AUTO })
        const result = await worker.recognize(image)
        texts.push(result.data.text || '')
        confidence = Math.min(confidence, Number(result.data.confidence || 0))
      } finally {
        await worker.terminate()
      }
    }
    process.stdout.write(JSON.stringify({ ok: true, text: texts.join('\n'), confidence }) + '\n')
  } catch (error) {
    process.stdout.write(JSON.stringify({ ok: false, error: String(error?.message || error) }) + '\n')
  }
}
