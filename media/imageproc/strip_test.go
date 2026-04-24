package imageproc

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// --- Test JPEG builder helpers ---
// --- テスト用 JPEG ビルダーヘルパー ---

// buildTestJPEG creates a minimal valid JPEG with optional EXIF containing
// orientation and/or GPS data. Returns raw JPEG bytes.
// Orientation と GPS データを含むオプションの EXIF 付き最小 JPEG を生成する。
func buildTestJPEG(orientation uint16, withGPS bool) []byte {
	// Encode a 2x2 red image as JPEG.
	// 2x2 の赤い画像を JPEG にエンコードする。
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90})
	raw := jpegBuf.Bytes()

	if orientation == 0 && !withGPS {
		// No EXIF requested — return plain JPEG.
		// EXIF 不要 — プレーン JPEG を返す。
		return raw
	}

	// Build minimal EXIF APP1 segment and inject after SOI (FF D8).
	// 最小 EXIF APP1 セグメントを構築し、SOI (FF D8) の直後に挿入する。
	exif := buildMinimalEXIF(orientation, withGPS)
	app1 := buildAPP1(exif)

	var out bytes.Buffer
	out.Write(raw[:2]) // SOI (FF D8)
	out.Write(app1)
	out.Write(raw[2:]) // Rest of JPEG
	return out.Bytes()
}

// buildMinimalEXIF constructs a minimal EXIF payload (little-endian TIFF).
// 最小 EXIF ペイロード (リトルエンディアン TIFF) を構築する。
func buildMinimalEXIF(orientation uint16, withGPS bool) []byte {
	var buf bytes.Buffer

	// EXIF header: "Exif\0\0"
	buf.WriteString("Exif\x00\x00")

	// TIFF header (little-endian). / TIFF ヘッダ (リトルエンディアン)。
	tiffStart := buf.Len()
	buf.WriteString("II")                                      // Byte order: little-endian
	binary.Write(&buf, binary.LittleEndian, uint16(0x002A))    // TIFF magic
	binary.Write(&buf, binary.LittleEndian, uint32(8))         // Offset to IFD0 (right after TIFF header)

	// IFD0: count entries. / IFD0: エントリ数。
	var entryCount uint16
	if orientation > 0 {
		entryCount++
	}
	if withGPS {
		entryCount++ // GPS IFD pointer
	}
	binary.Write(&buf, binary.LittleEndian, entryCount)

	// Calculate offset for GPS IFD data (after IFD0 + next IFD pointer).
	// GPS IFD データのオフセット (IFD0 + 次の IFD ポインタの後)。
	ifd0Size := 2 + int(entryCount)*12 + 4 // count + entries + next_ifd_offset
	gpsIFDOffset := uint32(8 + ifd0Size)    // relative to TIFF start

	// Orientation entry (tag 0x0112, type SHORT, count 1).
	// Orientation エントリ (タグ 0x0112、型 SHORT、カウント 1)。
	if orientation > 0 {
		binary.Write(&buf, binary.LittleEndian, uint16(0x0112)) // Tag
		binary.Write(&buf, binary.LittleEndian, uint16(3))      // Type: SHORT
		binary.Write(&buf, binary.LittleEndian, uint32(1))      // Count
		binary.Write(&buf, binary.LittleEndian, uint16(orientation)) // Value
		binary.Write(&buf, binary.LittleEndian, uint16(0))      // Padding
	}

	// GPS IFD pointer entry (tag 0x8825, type LONG, count 1).
	// GPS IFD ポインタエントリ (タグ 0x8825、型 LONG、カウント 1)。
	if withGPS {
		binary.Write(&buf, binary.LittleEndian, uint16(0x8825)) // Tag: GPSInfoIFDPointer
		binary.Write(&buf, binary.LittleEndian, uint16(4))      // Type: LONG
		binary.Write(&buf, binary.LittleEndian, uint32(1))      // Count
		binary.Write(&buf, binary.LittleEndian, gpsIFDOffset)   // Value: offset to GPS IFD
	}

	// Next IFD offset = 0 (no more IFDs). / 次の IFD オフセット = 0。
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// GPS IFD with a dummy latitude entry.
	// ダミー緯度エントリ付き GPS IFD。
	if withGPS {
		binary.Write(&buf, binary.LittleEndian, uint16(1)) // 1 entry

		// GPSLatitudeRef (tag 0x0001, type ASCII, count 2, value "N\0")
		binary.Write(&buf, binary.LittleEndian, uint16(0x0001)) // Tag
		binary.Write(&buf, binary.LittleEndian, uint16(2))      // Type: ASCII
		binary.Write(&buf, binary.LittleEndian, uint32(2))      // Count
		buf.WriteString("N\x00")                                // Value inline
		binary.Write(&buf, binary.LittleEndian, uint16(0))      // Padding

		// Next IFD offset = 0.
		binary.Write(&buf, binary.LittleEndian, uint32(0))
	}

	// Verify TIFF data starts at expected position.
	// TIFF データが期待位置から始まることを確認。
	_ = tiffStart

	return buf.Bytes()
}

// buildAPP1 wraps an EXIF payload into a JPEG APP1 marker segment.
// EXIF ペイロードを JPEG APP1 マーカーセグメントにラップする。
func buildAPP1(exif []byte) []byte {
	length := uint16(len(exif) + 2) // +2 for length field itself
	var buf bytes.Buffer
	buf.WriteByte(0xFF)
	buf.WriteByte(0xE1) // APP1 marker
	binary.Write(&buf, binary.BigEndian, length)
	buf.Write(exif)
	return buf.Bytes()
}

// buildTestPNG creates a minimal valid PNG.
// 最小の有効な PNG を生成する。
func buildTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// --- Helper: find APP1 markers in JPEG ---
// --- ヘルパー: JPEG 内の APP1 マーカーを探す ---

// hasAPP1 returns true if the JPEG contains an APP1 (EXIF) marker.
// JPEG が APP1 (EXIF) マーカーを含む場合 true を返す。
func hasAPP1(data []byte) bool {
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF && data[i+1] == 0xE1 {
			return true
		}
	}
	return false
}

// readOrientation extracts the Orientation value from EXIF APP1 if present.
// Returns 0 if not found.
// EXIF APP1 から Orientation 値を取得する。見つからなければ 0 を返す。
func readOrientation(data []byte) uint16 {
	for i := 0; i < len(data)-1; i++ {
		if data[i] != 0xFF || data[i+1] != 0xE1 {
			continue
		}
		if i+4 >= len(data) {
			return 0
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if i+4+segLen-2 > len(data) {
			return 0
		}
		seg := data[i+4 : i+2+segLen]
		// Check "Exif\0\0" magic.
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
	return 0
}

// hasGPSData checks if the EXIF contains GPS IFD pointer (tag 0x8825).
// EXIF に GPS IFD ポインタ (タグ 0x8825) が含まれるか確認する。
func hasGPSData(data []byte) bool {
	for i := 0; i < len(data)-1; i++ {
		if data[i] != 0xFF || data[i+1] != 0xE1 {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if i+4+segLen-2 > len(data) {
			return false
		}
		seg := data[i+4 : i+2+segLen]
		if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
			return false
		}
		tiff := seg[6:]
		var bo binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			bo = binary.LittleEndian
		case "MM":
			bo = binary.BigEndian
		default:
			return false
		}
		ifdOffset := bo.Uint32(tiff[4:8])
		if int(ifdOffset)+2 > len(tiff) {
			return false
		}
		count := bo.Uint16(tiff[ifdOffset : ifdOffset+2])
		for j := 0; j < int(count); j++ {
			entryOff := int(ifdOffset) + 2 + j*12
			if entryOff+12 > len(tiff) {
				break
			}
			tag := bo.Uint16(tiff[entryOff : entryOff+2])
			if tag == 0x8825 { // GPSInfoIFDPointer
				return true
			}
		}
		return false
	}
	return false
}

// --- Tests ---

// TestStripEXIF_OrientationNormal_WithGPS verifies that GPS is removed and
// no EXIF is written back when orientation is 1 (normal).
// Orientation=1 で GPS 付きの場合、GPS が除去され EXIF が書き戻されないことを確認する。
func TestStripEXIF_OrientationNormal_WithGPS(t *testing.T) {
	input := buildTestJPEG(1, true)

	// Verify test input has EXIF with GPS. / テスト入力に EXIF+GPS があることを確認。
	if !hasAPP1(input) {
		t.Fatal("test input should have APP1")
	}
	if !hasGPSData(input) {
		t.Fatal("test input should have GPS data")
	}

	result, err := StripEXIF(input)
	if err != nil {
		t.Fatalf("StripEXIF failed: %v", err)
	}

	// Should be valid JPEG. / 有効な JPEG であること。
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("result is not a valid JPEG")
	}

	// GPS should be gone. / GPS が消えていること。
	if hasGPSData(result) {
		t.Error("GPS data should be stripped")
	}

	// Orientation 1 needs no write-back — no APP1 at all is acceptable.
	// Orientation 1 は書き戻し不要 — APP1 なしでも OK。
	if hasAPP1(result) {
		ori := readOrientation(result)
		if ori != 0 && ori != 1 {
			t.Errorf("unexpected orientation in result: %d", ori)
		}
	}

	// Image should still be decodable. / 画像がデコード可能であること。
	if _, err := jpeg.Decode(bytes.NewReader(result)); err != nil {
		t.Fatalf("result JPEG is not decodable: %v", err)
	}
}

// TestStripEXIF_OrientationRotated_WithGPS verifies that GPS is removed and
// orientation is preserved when rotation is needed.
// 回転が必要な場合、GPS が除去され Orientation が保持されることを確認する。
func TestStripEXIF_OrientationRotated_WithGPS(t *testing.T) {
	input := buildTestJPEG(6, true) // 90° CW rotation

	result, err := StripEXIF(input)
	if err != nil {
		t.Fatalf("StripEXIF failed: %v", err)
	}

	// GPS should be gone. / GPS が消えていること。
	if hasGPSData(result) {
		t.Error("GPS data should be stripped")
	}

	// Orientation 6 should be written back. / Orientation 6 が書き戻されていること。
	ori := readOrientation(result)
	if ori != 6 {
		t.Errorf("orientation should be 6, got %d", ori)
	}

	// Image should still be decodable. / 画像がデコード可能であること。
	if _, err := jpeg.Decode(bytes.NewReader(result)); err != nil {
		t.Fatalf("result JPEG is not decodable: %v", err)
	}
}

// TestStripEXIF_NoEXIF verifies that a JPEG without EXIF passes through unchanged.
// EXIF なし JPEG がそのまま素通りすることを確認する。
func TestStripEXIF_NoEXIF(t *testing.T) {
	input := buildTestJPEG(0, false)

	if hasAPP1(input) {
		t.Fatal("test input should not have APP1")
	}

	result, err := StripEXIF(input)
	if err != nil {
		t.Fatalf("StripEXIF failed: %v", err)
	}

	// Should be identical to input. / 入力と同一であること。
	if !bytes.Equal(result, input) {
		t.Error("JPEG without EXIF should pass through unchanged")
	}
}

// TestStripEXIF_PNG verifies that PNG passes through unchanged.
// PNG がそのまま素通りすることを確認する。
func TestStripEXIF_PNG(t *testing.T) {
	input := buildTestPNG()

	result, err := StripEXIF(input)
	if err != nil {
		t.Fatalf("StripEXIF failed: %v", err)
	}

	// Should be identical to input. / 入力と同一であること。
	if !bytes.Equal(result, input) {
		t.Error("PNG should pass through unchanged")
	}
}

// TestStripEXIF_PreservesAPP0andAPP2 verifies that JFIF (APP0) and ICC (APP2)
// markers are preserved while APP1 is removed.
// JFIF (APP0) と ICC (APP2) マーカーが保持され APP1 が除去されることを確認する。
func TestStripEXIF_PreservesAPP0andAPP2(t *testing.T) {
	// Build JPEG with APP0 + APP1 + APP2. / APP0 + APP1 + APP2 付き JPEG を構築。
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var jpegBuf bytes.Buffer
	jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90})
	raw := jpegBuf.Bytes()

	// Inject APP0, APP1 (EXIF), APP2 (fake ICC) after SOI.
	// SOI の後に APP0, APP1, APP2 を挿入。
	var built bytes.Buffer
	built.Write(raw[:2]) // SOI

	// APP0 (JFIF marker). / APP0 (JFIF マーカー)。
	built.Write([]byte{0xFF, 0xE0, 0x00, 0x05, 'J', 'F', 'I'})

	// APP1 (EXIF). / APP1 (EXIF)。
	exif := buildMinimalEXIF(3, true) // Orientation=3 + GPS
	built.Write(buildAPP1(exif))

	// APP2 (fake ICC profile marker). / APP2 (ダミー ICC プロファイルマーカー)。
	built.Write([]byte{0xFF, 0xE2, 0x00, 0x06, 'I', 'C', 'C', '_'})

	built.Write(raw[2:]) // Rest

	input := built.Bytes()

	result, err := StripEXIF(input)
	if err != nil {
		t.Fatalf("StripEXIF failed: %v", err)
	}

	// APP1 should be gone. GPS should be gone.
	// APP1 が消えていること。GPS が消えていること。
	if hasGPSData(result) {
		t.Error("GPS data should be stripped")
	}

	// APP0 should still be present. / APP0 が残っていること。
	hasAPP0 := false
	for i := 0; i < len(result)-1; i++ {
		if result[i] == 0xFF && result[i+1] == 0xE0 {
			hasAPP0 = true
			break
		}
	}
	if !hasAPP0 {
		t.Error("APP0 (JFIF) should be preserved")
	}

	// APP2 should still be present. / APP2 が残っていること。
	hasAPP2 := false
	for i := 0; i < len(result)-1; i++ {
		if result[i] == 0xFF && result[i+1] == 0xE2 {
			hasAPP2 = true
			break
		}
	}
	if !hasAPP2 {
		t.Error("APP2 (ICC) should be preserved")
	}
}

func TestStripEXIF_MalformedSegLen(t *testing.T) {
	// L10: segLen < 2 の不正 JPEG でパニックや無限ループにならないことを検証。
	// L10: Verify malformed JPEG with segLen < 2 doesn't panic or loop.

	// SOI + marker with segLen=0 (2 bytes for length field, but value is 0).
	// SOI + segLen=0 のマーカー。
	malformed := []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xE0, // APP0 marker
		0x00, 0x00, // segLen = 0 (invalid)
		0xFF, 0xDA, // SOS (image data start)
		0x00, 0x01, // fake data
	}

	result, err := StripEXIF(malformed)
	if err != nil {
		t.Fatalf("StripEXIF: %v", err)
	}
	// Should not panic, should return data.
	// パニックせず、データを返すこと。
	if len(result) == 0 {
		t.Error("StripEXIF returned empty for malformed JPEG")
	}
}
