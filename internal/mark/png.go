package mark

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
)

// EncodePNG кодирует картинку и вставляет текстовые чанки после IHDR:
// iTXt StudyMark (JSON), tEXt StudyMarkASCII, tEXt StudyID, iTXt StudyWindows — как PngInfo в Linux-версии.
// PIL читает их в Image.text, study-mark-verify.py берёт StudyMark и StudyWindows.
func EncodePNG(im image.Image, markJSON, asciiJSON []byte, id string, windowsJSON []byte) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, im); err != nil {
		return nil, err
	}
	raw := buf.Bytes()
	if len(raw) < 8+8+13+4 || string(raw[:8]) != "\x89PNG\r\n\x1a\n" {
		return nil, fmt.Errorf("png: неожиданный заголовок")
	}
	ihdrLen := int(binary.BigEndian.Uint32(raw[8:12]))
	cut := 8 + 8 + ihdrLen + 4 // конец чанка IHDR
	out := bytes.NewBuffer(make([]byte, 0, len(raw)+len(markJSON)+len(asciiJSON)+len(windowsJSON)+256))
	out.Write(raw[:cut])
	out.Write(chunk("iTXt", itxt("StudyMark", markJSON)))
	out.Write(chunk("tEXt", text("StudyMarkASCII", asciiJSON)))
	out.Write(chunk("tEXt", text("StudyID", []byte(id))))
	out.Write(chunk("iTXt", itxt("StudyWindows", windowsJSON)))
	out.Write(raw[cut:])
	return out.Bytes(), nil
}

func chunk(typ string, data []byte) []byte {
	b := make([]byte, 0, 12+len(data))
	b = binary.BigEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, typ...)
	b = append(b, data...)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	return binary.BigEndian.AppendUint32(b, crc.Sum32())
}

// itxt: keyword\0 compression_flag=0 compression_method=0 language\0 translated\0 text(UTF-8)
func itxt(key string, data []byte) []byte {
	b := []byte(key)
	b = append(b, 0, 0, 0, 0, 0)
	return append(b, data...)
}

// text: keyword\0 text(Latin-1; у нас только ASCII)
func text(key string, data []byte) []byte {
	b := []byte(key)
	b = append(b, 0)
	return append(b, data...)
}
