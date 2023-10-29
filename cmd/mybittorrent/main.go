package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"

	bencode "github.com/jackpal/bencode-go"
	"golang.org/x/sync/errgroup"
)

func main() {
	switch cmd := os.Args[1]; cmd {
	case "decode":
		input := os.Args[2]
		val, tail, err := decodeBencode(input)
		if err != nil {
			fmt.Printf("%s: %s", tail, err)
			return
		}
		if tail != "" {
			fmt.Printf("didn't consume the whole input: tail %q", tail)
			return
		}

		out, _ := json.Marshal(val)
		fmt.Println(string(out))
	case "info":
		filePath := os.Args[2]
		f, err := os.Open(filePath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		var t Tracker
		err = bencode.Unmarshal(f, &t)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Tracker URL: %s\n", t.Announce)
		fmt.Printf("Length: %d\n", t.Info.Length)

		infoHash, err := t.InfoHash()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Info Hash: %x\n", infoHash)
		fmt.Printf("Piece Length: %d\n", t.Info.PieceLength)

		fmt.Printf("Piece Hashes:\n")
		piecesIter := t.Info.PiecesAll()
		piecesIter(func(_ int, piece []byte) bool {
			fmt.Printf("%x\n", piece)
			return true
		})
	case "peers":
		filePath := os.Args[2]
		f, err := os.Open(filePath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		var t Tracker
		err = bencode.Unmarshal(f, &t)
		if err != nil {
			panic(err)
		}

		peers, err := discoverPeers(t)
		if err != nil {
			panic(err)
		}

		for _, peer := range peers {
			fmt.Println(peer.String())
		}
	case "handshake":
		filePath := os.Args[2]
		peer := os.Args[3]

		f, err := os.Open(filePath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		var t Tracker
		err = bencode.Unmarshal(f, &t)
		if err != nil {
			panic(err)
		}

		conn, err := net.Dial("tcp", peer)
		if err != nil {
			panic(err)
		}
		defer conn.Close()

		infoHash, err := t.InfoHash()
		if err != nil {
			panic(err)
		}

		hsk := newHandshake(infoHash)

		_, err = hsk.WriteTo(conn)
		if err != nil {
			panic(err)
		}

		_, err = hsk.ReadFrom(conn)
		if err != nil {
			panic(err)
		}

		if hsk.Tag != 19 {
			panic("unexpected handshake response")
		}

		fmt.Printf("Peer ID: %x\n", hsk.PeerID)
	case "download_piece":
		err := downloadPieceCmd(os.Args[2:])
		if err != nil {
			fmt.Println("Download piece command failed: " + err.Error())
			os.Exit(1)
		}
	case "download":
		err := downloadCmd(os.Args[2:])
		if err != nil {
			fmt.Println("Download command failed: " + err.Error())
			os.Exit(1)
		}
	default:
		fmt.Println("Unknown command: " + cmd)
		os.Exit(1)
	}
}

func downloadPieceCmd(args []string) error {
	flags := flag.NewFlagSet("download_piece", flag.ExitOnError)

	var outPath string
	flags.StringVar(&outPath, "o", "", "Ouput path")

	if err := flags.Parse(args); err != nil {
		return err
	}

	filePath := flags.Arg(0)
	piece, _ := strconv.Atoi(flags.Arg(1))

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var t Tracker
	err = bencode.Unmarshal(f, &t)
	if err != nil {
		return err
	}

	peers, err := discoverPeers(t)
	if err != nil {
		return err
	}

	conn, err := net.Dial("tcp", peers[0].String())
	if err != nil {
		return err
	}
	defer conn.Close()

	peer := NewPeer(conn)

	err = handshakePeer(t, peer)
	if err != nil {
		return err
	}

	if payload, err := peer.Recv(Bitfield); err != nil {
		return nil
	} else {
		// TODO: check that bitfield's payload has the len(pieces) bits set
		fmt.Printf("bitfield - %d\n", payload)
	}

	if err := peer.Send(Interested, nil); err != nil {
		return nil
	}

	if payload, err := peer.Recv(Unchoke); err != nil {
		return nil
	} else {
		fmt.Printf("unchoke - %d\n", payload)
	}

	pf, err := os.CreateTemp("", filePath)
	if err != nil {
		return err
	}
	defer pf.Close()

	_, err = downloadPiece(peer, t, piece, pf)
	if err != nil {
		return err
	}

	pf.Seek(0, io.SeekStart)

	h := sha1.New()
	if _, err := io.Copy(h, pf); err != nil {
		return err
	}

	fmt.Printf("Piece hash: %x\n", h.Sum(nil))

	if err := os.Rename(pf.Name(), outPath); err != nil {
		os.Remove(pf.Name())
		return err
	}

	fmt.Printf("Piece %d downloaded to %s.\n", piece, outPath)

	return nil
}

func downloadCmd(args []string) error {
	flags := flag.NewFlagSet("download_piece", flag.ExitOnError)

	var outPath string
	flags.StringVar(&outPath, "o", "", "Ouput path")

	if err := flags.Parse(args); err != nil {
		return err
	}

	torrentFile := flags.Arg(0)
	t, err := newTrackerFromPath(torrentFile)
	if err != nil {
		return err
	}

	peersAddr, err := discoverPeers(t)
	if err != nil {
		return err
	}

	infoHash, err := t.InfoHash()
	if err != nil {
		return err
	}

	peers := make([]*Peer, len(peersAddr))
	for i, addr := range peersAddr {
		conn, err := net.Dial("tcp", addr.String())
		if err != nil {
			return err
		}
		defer conn.Close()

		peer := NewPeer(conn)

		peerID, err := peer.Handshake(infoHash)
		if err != nil {
			return err
		}

		if payload, err := peer.Recv(Bitfield); err != nil {
			return nil
		} else {
			fmt.Printf("peer %x (%s): bitfield - %d\n", peerID, addr, payload)
		}

		if err := peer.Send(Interested, nil); err != nil {
			return nil
		}
		if _, err := peer.Recv(Unchoke); err != nil {
			return nil
		}

		peers[i] = peer
	}

	f, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Truncate(int64(t.Info.Length)); err != nil {
		return err
	}

	var g errgroup.Group
	// to make it simpler, limit the concurrency with number of peers;
	// this makes sure a peer only responsible for one piece at the time
	g.SetLimit(len(peers))

	piecesIter := t.Info.PiecesAll()
	piecesIter(func(n int, pieceHash []byte) bool {
		peer := peers[n%len(peers)]

		g.Go(func() error {
			baseOff := int64(uint64(n) * t.Info.PieceLength)
			pw := io.NewOffsetWriter(f, baseOff)
			plen, err := downloadPiece(peer, t, n, pw)
			if err != nil {
				return err
			}

			h := sha1.New()
			if _, err := io.Copy(h, io.NewSectionReader(f, baseOff, plen)); err != nil {
				return err
			}
			gotHash := h.Sum(nil)
			if !bytes.Equal(pieceHash, gotHash) {
				return fmt.Errorf("malformed piece %d: want hash %x, got %x", n, pieceHash, gotHash)
			}

			fmt.Printf("piece %d - %x\n", n, pieceHash)

			return nil
		})

		return true
	})

	if err := g.Wait(); err != nil {
		os.Remove(f.Name())
		return err
	}

	if err := os.Rename(f.Name(), outPath); err != nil {
		return err
	}

	fmt.Printf("Downloaded %s to %s.\n", torrentFile, outPath)

	return nil
}

func downloadPiece(peer *Peer, t Tracker, piece int, pw io.WriterAt) (int64, error) {
	// block per piece rounded up
	blocksPerPiece := int((t.Info.PieceLength + maxBlockSize - 1) / maxBlockSize)

	for b := 0; b < blocksPerPiece; b++ {
		begin := uint32(b * maxBlockSize)
		blen := uint32(maxBlockSize)

		// the very last block (across all pieces) can be truncated, if file's length
		// doesn't perfectly align to the size of a block
		if total := uint64((piece + 1) * (b + 1) * int(blen)); total > t.Info.Length {
			blen = blen - uint32(total-t.Info.Length)
		}

		if err := peer.Request(piece, begin, blen); err != nil {
			return 0, err
		}
	}

	var plen int64
	for b := blocksPerPiece; b > 0; b-- {
		payload, err := peer.Recv(Piece)
		if err != nil {
			return 0, err
		}

		if p := binary.BigEndian.Uint32(payload[:]); p != uint32(piece) {
			return 0, fmt.Errorf("unexpected piece index %d", p)
		}

		begin := binary.BigEndian.Uint32(payload[4:])
		sz, err := pw.WriteAt(payload[8:], int64(begin))
		if err != nil {
			return 0, err
		}
		plen += int64(sz)
	}

	return plen, nil
}

func discoverPeers(t Tracker) ([]netip.AddrPort, error) {
	turl, err := trackerURLFrom(t)
	if err != nil {
		return nil, err
	}

	httpResp, err := http.Get(turl)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var resp trackerResponse
	err = bencode.Unmarshal(httpResp.Body, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Peers(), nil
}

func handshakePeer(t Tracker, peer *Peer) error {
	infoHash, err := t.InfoHash()
	if err != nil {
		return err
	}

	_, err = peer.Handshake(infoHash)
	return err
}

type Tracker struct {
	// Announce is a URL to a tracker.
	Announce string
	// Info contains metainfo of a tracker.
	Info TrackerInfo
}

func newTrackerFromPath(path string) (t Tracker, err error) {
	f, err := os.Open(path)
	if err != nil {
		return t, err
	}
	defer f.Close()

	err = bencode.Unmarshal(f, &t)
	return t, err
}

type TrackerInfo struct {
	// Name is a suggested name to save the file or directory as.
	Name string `bencode:"name"`
	// PieceLength is the number of bytes in each piece the file is split into.
	PieceLength uint64 `bencode:"piece length"`
	// Pieces is a string of multiple of 20. It is to be subdivided into strings of length 20,
	// each of which is the SHA1 hash of the piece at the corresponding index.
	Pieces string `bencode:"pieces"`
	// Length is the size of the file in bytes, for single-file torrents
	Length uint64 `bencode:"length"`
}

func (info TrackerInfo) PiecesAll() func(func(int, []byte) bool) {
	return func(yield func(int, []byte) bool) {
		var n int
		for p := []byte(info.Pieces); len(p) > 0; p = p[20:] {
			if !yield(n, p[:20]) {
				return
			}
			n++
		}
	}
}

func (info TrackerInfo) PiecesTotal() int {
	return len(info.Pieces) % 20
}

func (t Tracker) InfoHash() ([]byte, error) {
	h := sha1.New()
	err := bencode.Marshal(h, t.Info)
	if err != nil {
		return nil, err
	}
	ret := h.Sum(nil)
	return ret[:], nil
}

func trackerURLFrom(t Tracker) (string, error) {
	u, err := url.Parse(t.Announce)
	if err != nil {
		return "", nil
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", nil
	}
	// the info hash of the torrent file
	infoHash, err := t.InfoHash()
	if err != nil {
		return "", nil
	}
	q.Add("info_hash", string(infoHash))
	// a unique identifier for your client
	q.Add("peer_id", "00112233445566778899")
	// the port your client is listening on
	q.Add("port", "6881")
	// the total amount uploaded so far (always 0)
	q.Add("uploaded", "0")
	// the total amount downloaded (always 0)
	q.Add("downloaded", "0")
	// the number of bytes left to download
	left := strconv.FormatUint(t.Info.Length, 10)
	q.Add("left", left)
	// whether the peer list should use the compact representation (always 1)
	q.Add("compact", "1")

	u.RawQuery = q.Encode()

	return u.String(), nil
}

type trackerResponse struct {
	Interval uint64 `bencode:"interval"`
	RawPeers string `bencode:"peers"`
}

func (tr trackerResponse) Peers() []netip.AddrPort {
	rawPeers := []byte(tr.RawPeers)
	peers := make([]netip.AddrPort, 0, len(rawPeers)/6)
	for len(rawPeers) > 0 {
		addr, _ := netip.AddrFromSlice(rawPeers[:4])
		port := binary.BigEndian.Uint16(rawPeers[4:])
		peers = append(peers, netip.AddrPortFrom(addr, port))
		rawPeers = rawPeers[6:]
	}
	return peers
}
