// Package imageproc provides image processing utilities for media uploads.
// メディアアップロード用の画像処理ユーティリティを提供するパッケージ。
package imageproc

import (
	"bytes"
	"encoding/binary"
)

// StripEXIF removes EXIF (APP1) metadata from JPEG images while preserving
// the Orientation tag if it indicates rotation. APP0 (JFIF), APP2 (ICC),
// and all other markers are preserved. Non-JPEG images pass through unchanged.
//
// JPEG 画像から EXIF (APP1) メタデータを除去する。回転を示す Orientation タグは
// 保持する。APP0 (JFIF)、APP2 (ICC) 等のマーカーはそのまま残す。
// JPEG 以外の画像は変更せずに返す。
func StripEXIF(data []byte) ([]byte, error) {
	// Not a JPEG — pass through. / JPEG でなければ素通り。
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return data, nil
	}

	// Scan for APP1 markers and read orientation before modifying.
	// 変更前に APP1 マーカーをスキャンして Orientation を読み取る。
	orientation := findOrientation(data)

	// Rebuild JPEG without APP1 markers.
	// APP1 マーカーなしで JPEG を再構築する。
	result := removeAPP1Markers(data)

	// If no APP1 was found, return original unchanged.
	// APP1 が見つからなければ元のまま返す。
	if len(result) == len(data) {
		return data, nil
	}

	// Write back orientation if rotation is needed (not 0 or 1).
	// 回転が必要な場合 (0, 1 以外) Orientation を書き戻す。
	if orientation > 1 {
		result = injectOrientationAPP1(result, orientation)
	}

	return result, nil
}

// findOrientation extracts the EXIF Orientation value from the first APP1 marker.
// Returns 0 if not found.
// 最初の APP1 マーカーから EXIF Orientation 値を取得する。見つからなければ 0 を返す。
func findOrientation(data []byte) uint16 {
	i := 2 // Skip SOI (FF D8). / SOI をスキップ。
	for i+3 < len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]

		// SOS (FF DA) — image data starts, stop scanning.
		// SOS (FF DA) — 画像データ開始、スキャン終了。
		if marker == 0xDA {
			break
		}

		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 {
			break
		}
		segEnd := i + 2 + segLen
		if segEnd > len(data) {
			break
		}

		if marker == 0xE1 { // APP1
			return parseOrientationFromEXIF(data[i+4 : segEnd])
		}

		i = segEnd
	}
	return 0
}

// parseOrientationFromEXIF parses an EXIF payload and returns the Orientation tag value.
// EXIF ペイロードをパースして Orientation タグ値を返す。
func parseOrientationFromEXIF(seg []byte) uint16 {
	// Check "Exif\0\0" magic. / "Exif\0\0" マジックを確認。
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}
	tiff := seg[6:]

	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}

	if bo.Uint16(tiff[2:4]) != 0x002A {
		return 0
	}

	ifdOffset := bo.Uint32(tiff[4:8])
	if int(ifdOffset)+2 > len(tiff) {
		return 0
	}

	count := bo.Uint16(tiff[ifdOffset : ifdOffset+2])
	for j := 0; j < int(count); j++ {
		entryOff := int(ifdOffset) + 2 + j*12
		if entryOff+12 > len(tiff) {
			break
		}
		tag := bo.Uint16(tiff[entryOff : entryOff+2])
		if tag == 0x0112 { // Orientation
			return bo.Uint16(tiff[entryOff+8 : entryOff+10])
		}
	}
	return 0
}

// removeAPP1Markers returns a copy of the JPEG with all APP1 (FF E1) markers removed.
// 全 APP1 (FF E1) マーカーを除去した JPEG のコピーを返す。
func removeAPP1Markers(data []byte) []byte {
	var buf bytes.Buffer
	buf.Write(data[:2]) // SOI (FF D8)

	i := 2
	for i+3 < len(data) {
		if data[i] != 0xFF {
			// Not a marker — write rest and break.
			// マーカーでない — 残りを書き出して終了。
			buf.Write(data[i:])
			break
		}
		marker := data[i+1]

		// SOS (FF DA) — rest is image data, copy everything.
		// SOS (FF DA) — 残りは画像データ、全てコピー。
		if marker == 0xDA {
			buf.Write(data[i:])
			break
		}

		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 {
			buf.Write(data[i:])
			break
		}
		segEnd := i + 2 + segLen
		if segEnd > len(data) {
			buf.Write(data[i:])
			break
		}

		if marker == 0xE1 {
			// Skip APP1 — don't write. / APP1 をスキップ — 書き出さない。
			i = segEnd
			continue
		}

		// Keep this marker segment. / このマーカーセグメントを保持。
		buf.Write(data[i:segEnd])
		i = segEnd
	}

	return buf.Bytes()
}

// injectOrientationAPP1 inserts a minimal APP1 (EXIF) segment containing only
// the Orientation tag right after the SOI marker.
// SOI マーカー直後に Orientation タグのみを含む最小 APP1 (EXIF) セグメントを挿入する。
func injectOrientationAPP1(data []byte, orientation uint16) []byte {
	// Build minimal EXIF: "Exif\0\0" + TIFF header + IFD0 with 1 entry.
	// 最小 EXIF: "Exif\0\0" + TIFF ヘッダ + 1エントリの IFD0。
	var exif bytes.Buffer

	// EXIF header. / EXIF ヘッダ。
	exif.WriteString("Exif\x00\x00")

	// TIFF header (little-endian). / TIFF ヘッダ (リトルエンディアン)。
	exif.WriteString("II")
	binary.Write(&exif, binary.LittleEndian, uint16(0x002A))
	binary.Write(&exif, binary.LittleEndian, uint32(8)) // IFD0 offset

	// IFD0: 1 entry (Orientation). / IFD0: 1 エントリ (Orientation)。
	binary.Write(&exif, binary.LittleEndian, uint16(1)) // Count

	// Orientation entry: tag=0x0112, type=SHORT(3), count=1, value=orientation.
	binary.Write(&exif, binary.LittleEndian, uint16(0x0112))
	binary.Write(&exif, binary.LittleEndian, uint16(3))
	binary.Write(&exif, binary.LittleEndian, uint32(1))
	binary.Write(&exif, binary.LittleEndian, orientation)
	binary.Write(&exif, binary.LittleEndian, uint16(0)) // Padding

	// Next IFD offset = 0. / 次の IFD オフセット = 0。
	binary.Write(&exif, binary.LittleEndian, uint32(0))

	// Wrap in APP1 marker. / APP1 マーカーでラップ。
	exifBytes := exif.Bytes()
	app1Len := uint16(len(exifBytes) + 2) // +2 for length field

	var app1 bytes.Buffer
	app1.WriteByte(0xFF)
	app1.WriteByte(0xE1)
	binary.Write(&app1, binary.BigEndian, app1Len)
	app1.Write(exifBytes)

	// Insert after SOI. / SOI の後に挿入。
	var result bytes.Buffer
	result.Write(data[:2]) // SOI
	result.Write(app1.Bytes())
	result.Write(data[2:]) // Rest
	return result.Bytes()
}
