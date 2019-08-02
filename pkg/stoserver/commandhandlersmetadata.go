package stoserver

import (
	"errors"
	"fmt"
	"github.com/function61/eventkit/command"
	"github.com/function61/gokit/sliceutil"
	"github.com/function61/varasto/pkg/seasonepisodedetector"
	"github.com/function61/varasto/pkg/stoserver/stodb"
	"github.com/function61/varasto/pkg/stoserver/stoservertypes"
	"github.com/function61/varasto/pkg/stotypes"
	"github.com/function61/varasto/pkg/themoviedbapi"
	"go.etcd.io/bbolt"
	"strconv"
	"strings"
)

// this is for movies
func (c *cHandlers) CollectionPullMetadata(cmd *stoservertypes.CollectionPullMetadata, ctx *command.Ctx) error {
	tmdb, err := c.themoviedbapiClient()
	if err != nil {
		return err
	}

	return c.db.Update(func(tx *bolt.Tx) error {
		collection, err := stodb.Read(tx).Collection(cmd.Collection)
		if err != nil {
			return err
		}

		info, err := tmdb.OpenMovieByImdbId(cmd.ForeignKey)
		if err != nil {
			return err
		}

		// store because we might lose detail when scrubbing name
		if cmd.ScrubName {
			if collection.Name != info.OriginalTitle {
				collection.Metadata["previous_name"] = collection.Name
			}

			collection.Name = info.OriginalTitle
		}

		collection.Metadata[stoservertypes.MetadataTheMovieDbMovieId] = strconv.Itoa(int(info.Id))
		if info.ExternalIds.ImdbId != "" {
			collection.Metadata[stoservertypes.MetadataImdbId] = info.ExternalIds.ImdbId
		}
		if info.Overview != "" {
			collection.Metadata[stoservertypes.MetadataOverview] = info.Overview
		}
		if info.RuntimeMinutes != 0 {
			collection.Metadata[stoservertypes.MetadataVideoRuntimeMins] = strconv.Itoa(info.RuntimeMinutes)
		}
		if info.RevenueDollars != 0 {
			collection.Metadata[stoservertypes.MetadataVideoRevenueDollars] = strconv.Itoa(int(info.RevenueDollars))
		}
		if info.BackdropPath != "" {
			collection.Metadata[stoservertypes.MetadataBackdrop] = themoviedbapi.ImagePath(info.BackdropPath, "original")
		}
		if info.ReleaseDate != "" {
			collection.Metadata[stoservertypes.MetadataReleaseDate] = info.ReleaseDate
		}

		return stodb.CollectionRepository.Update(collection, tx)
	})
}

// directory holds a bunch of series
func (c *cHandlers) DirectoryPullMetadata(cmd *stoservertypes.DirectoryPullMetadata, ctx *command.Ctx) error {
	tmdb, err := c.themoviedbapiClient()
	if err != nil {
		return err
	}

	tv, err := tmdb.OpenTvByImdbId(cmd.ForeignKey)
	if err != nil {
		return err
	}

	return c.db.Update(func(tx *bolt.Tx) error {
		dir, err := stodb.Read(tx).Directory(cmd.Directory)
		if err != nil {
			return err
		}

		dir.Metadata[stoservertypes.MetadataTheMovieDbTvId] = fmt.Sprintf("%d", tv.Id)

		if tv.BackdropPath != "" {
			dir.Metadata[stoservertypes.MetadataBackdrop] = themoviedbapi.ImagePath(tv.BackdropPath, "original")
		}

		if tv.Overview != "" {
			dir.Metadata[stoservertypes.MetadataOverview] = tv.Overview
		}

		if tv.Homepage != "" {
			dir.Metadata[stoservertypes.MetadataHomepage] = tv.Homepage
		}

		if tv.ExternalIds.ImdbId != "" {
			dir.Metadata[stoservertypes.MetadataImdbId] = tv.ExternalIds.ImdbId
		}

		return stodb.DirectoryRepository.Update(dir, tx)
	})
}

// this is for serie episodes
func (c *cHandlers) CollectionRefreshMetadataAutomatically(cmd *stoservertypes.CollectionRefreshMetadataAutomatically, ctx *command.Ctx) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		// Collection is validated as non-empty
		collIds := strings.Split(cmd.Collection, ",")

		firstColl, err := stodb.Read(tx).Collection(collIds[0])
		if err != nil {
			return err
		}

		firstCollDirectory, err := stodb.Read(tx).Directory(firstColl.Directory)
		if err != nil {
			return err
		}

		parentDirs, err := getParentDirs(*firstCollDirectory, tx)
		if err != nil {
			return err
		}

		theTvDbSeriesId := ""
		for _, parentDir := range parentDirs {
			theTvDbSeriesId = parentDir.Metadata[stoservertypes.MetadataTheMovieDbTvId]
			if theTvDbSeriesId != "" {
				break
			}
		}
		if theTvDbSeriesId == "" {
			theTvDbSeriesId = firstCollDirectory.Metadata[stoservertypes.MetadataTheMovieDbTvId] // one last try
		}
		if theTvDbSeriesId == "" {
			return fmt.Errorf("could not resolve %s for collection", stoservertypes.MetadataTheMovieDbTvId)
		}

		uniqueSeasonNumbers := []int{}

		type episodeAndCollPair struct {
			seasonEpisode seasonepisodedetector.Result
			coll          *stotypes.Collection
		}

		pairs := []episodeAndCollPair{}

		findPair := func(seasonEpisode seasonepisodedetector.Result) *episodeAndCollPair {
			for _, pair := range pairs {
				if seasonEpisode.LaxEqual(pair.seasonEpisode) {
					return &pair
				}
			}

			return nil
		}

		for _, collId := range collIds {
			coll, err := stodb.Read(tx).Collection(collId)
			if err != nil {
				return err
			}

			if coll.Directory != firstColl.Directory {
				return errors.New("all input collections must be siblings in the directory hierarchy")
			}

			seasonEpisode := seasonepisodedetector.Detect(coll.Name)
			if seasonEpisode == nil {
				continue
			}

			pairs = append(pairs, episodeAndCollPair{*seasonEpisode, coll})

			seasonNumber, err := strconv.Atoi(seasonEpisode.Season)
			if err != nil {
				return err // should not happen
			}

			if !sliceutil.ContainsInt(uniqueSeasonNumbers, seasonNumber) {
				uniqueSeasonNumbers = append(uniqueSeasonNumbers, seasonNumber)
			}
		}

		tmdb, err := c.themoviedbapiClient()
		if err != nil {
			return err
		}

		for _, seasonNumber := range uniqueSeasonNumbers {
			episodes, err := tmdb.GetSeasonEpisodes(seasonNumber, theTvDbSeriesId)
			if err != nil {
				return err
			}

			for _, ep := range episodes {
				seasonEpisode := seasonepisodedetector.Result{
					Season:  fmt.Sprintf("%d", ep.SeasonNumber),
					Episode: fmt.Sprintf("%d", ep.EpisodeNumber),
				}

				pair := findPair(seasonEpisode)
				if pair == nil {
					continue
				}

				coll := pair.coll

				if coll.Metadata == nil {
					panic("should not be after migration")
				}

				coll.Metadata[stoservertypes.MetadataTheMovieDbTvId] = theTvDbSeriesId
				coll.Metadata[stoservertypes.MetadataTheMovieDbTvEpisodeId] = fmt.Sprintf("%d", ep.Id)

				if ep.Name != "" {
					coll.Metadata[stoservertypes.MetadataTitle] = ep.Name
				}
				if ep.AirDate != "" {
					coll.Metadata[stoservertypes.MetadataReleaseDate] = ep.AirDate
				}
				if ep.Overview != "" {
					coll.Metadata[stoservertypes.MetadataOverview] = ep.Overview
				}
				if ep.StillPath != "" {
					coll.Metadata[stoservertypes.MetadataThumbnail] = themoviedbapi.ImagePath(ep.StillPath, "original")
				}

				if err := stodb.CollectionRepository.Update(coll, tx); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (c *cHandlers) ConfigSetTheMovieDbApikey(cmd *stoservertypes.ConfigSetTheMovieDbApikey, ctx *command.Ctx) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		if cmd.Apikey != "" { // allow clearing this without testing
			// validate the API key by trying to use the API
			client := themoviedbapi.New(cmd.Apikey)
			_, err := client.OpenMovieByImdbId("tt1226229") // one of my fav underrated movies :)
			if err != nil {
				return fmt.Errorf("failed validating API key: %v", err)
			}
		}

		return stodb.CfgTheMovieDbApikey.Set(cmd.Apikey, tx)
	})
}

func (c *cHandlers) themoviedbapiClient() (*themoviedbapi.Client, error) {
	tx, err := c.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	apikey, err := stodb.CfgTheMovieDbApikey.GetRequired(tx)
	if err != nil {
		return nil, err
	}

	return themoviedbapi.New(apikey), nil
}